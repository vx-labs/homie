package homie

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/shirou/gopsutil/cpu"
	"github.com/sirupsen/logrus"
	"github.com/vx-labs/homie/config"
)

func NewClient(ctx context.Context, prefix string, server string, port int, mqttPrefix string, deviceName string, firmwareName string, logger *logrus.Entry) Client {
	store := config.NewStore(ctx, "/tmp/", logger)
	cfg := store.Get()
	if !cfg.Initialized {
		cfg.Mqtt.Host = server
		cfg.Mqtt.Port = port

		cfg.Mqtt.Prefix = mqttPrefix

		cfg.Homie.Prefix = prefix
		cfg.Homie.Name = deviceName
		cfg.Initialized = true
	}
	return &client{
		logger:          logger,
		cfgStore:        store,
		bootTime:        time.Now(),
		firmwareName:    firmwareName,
		nodes:           map[string]Node{},
		publishChan:     make(chan stateMessage, 10),
		subscribeChan:   make(chan subscribeMessage, 10),
		unsubscribeChan: make(chan unsubscribeMessage, 10),
		connected:       make(chan struct{}, 1),
		disconnected:    make(chan struct{}, 1),
		otaSessions:     map[string]OTASession{},
	}

}

func (homieClient *client) SetCustomTLSConfiguration(ssl_ca string, ssl_cert string, ssl_key string) {
	cfg := homieClient.cfgStore.Get()
	cfg.Mqtt.Ssl = true
	cfg.Mqtt.Ssl_Config.CA = ssl_ca
	cfg.Mqtt.Ssl_Config.ClientCert = ssl_cert
	cfg.Mqtt.Ssl_Config.Privkey = ssl_key

}
func (homieClient *client) EnableTLS() {
	cfg := homieClient.cfgStore.Get()
	cfg.Mqtt.Ssl = true
}
func (homieClient *client) SetCredentials(user, password string) {
	cfg := homieClient.cfgStore.Get()
	cfg.Mqtt.Username = user
	cfg.Mqtt.Password = password
}

func (homieClient *client) getMQTTOptions() *mqtt.ClientOptions {
	o := mqtt.NewClientOptions()
	o.Username = homieClient.cfgStore.Get().Mqtt.Username
	o.Password = homieClient.cfgStore.Get().Mqtt.Password
	o.AddBroker(homieClient.Url())
	o.SetClientID(homieClient.Id())
	o.SetWill(homieClient.getDevicePrefix()+"$online", "false", 1, true)
	o.SetKeepAlive(10 * time.Second)
	o.SetOnConnectHandler(homieClient.onConnectHandler)
	o.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		homieClient.logger.Errorf("connection to mqtt broker lost: %s", err.Error())
		homieClient.Disconnected() <- struct{}{}
	})
	if homieClient.cfgStore.Get().Mqtt.Ssl_Config.Privkey != "" {
		homieClient.logger.Debug("building TLS configuration")
		cert, err := tls.LoadX509KeyPair(homieClient.cfgStore.Get().Mqtt.Ssl_Config.ClientCert, homieClient.cfgStore.Get().Mqtt.Ssl_Config.Privkey)

		if err != nil {
			homieClient.logger.Error(err)
		} else {
			homieClient.logger.Debug("loaded TLS certificate and private key from ", homieClient.cfgStore.Get().Mqtt.Ssl_Config.ClientCert, " and ", homieClient.cfgStore.Get().Mqtt.Ssl_Config.Privkey)
			caCertPool := x509.NewCertPool()
			homieClient.logger.Debug("loading CA certificate from ", homieClient.cfgStore.Get().Mqtt.Ssl_Config.CA)
			caCert, err := ioutil.ReadFile(homieClient.cfgStore.Get().Mqtt.Ssl_Config.CA)
			if err != nil {
				homieClient.logger.Error(err)
			}
			caCertPool.AppendCertsFromPEM(caCert)
			loadedConfig := &tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true, RootCAs: caCertPool}
			o.SetTLSConfig(loadedConfig)
		}
	}
	return o
}

func (homieClient *client) Disconnected() chan struct{} {
	return homieClient.disconnected
}

func (homieClient *client) Connected() chan struct{} {
	return homieClient.connected
}

func (homieClient *client) publish(subtopic string, payload string) {
	if homieClient.mqttClient == nil {
		return
	}
	topic := homieClient.getDevicePrefix() + subtopic
	homieClient.logger.Debugf("%s <- %s", topic, payload)
	homieClient.mqttClient.Publish(topic, 1, true, payload)
}

func (homieClient *client) unsubscribe(subtopic string) {
	if homieClient.mqttClient == nil {
		return
	}
	topic := homieClient.getDevicePrefix() + subtopic
	homieClient.logger.Debugf("unsub: %s", topic)
	homieClient.mqttClient.Unsubscribe(topic)
}

func (homieClient *client) subscribe(subtopic string, callback func(path string, payload string)) {
	if homieClient.mqttClient == nil {
		return
	}
	topic := homieClient.getDevicePrefix() + subtopic
	homieClient.logger.Debugf("sub: %s", topic)
	homieClient.mqttClient.Subscribe(topic, 1, func(mqttClient mqtt.Client, mqttMessage mqtt.Message) {
		callback(mqttMessage.Topic(), string(mqttMessage.Payload()))
	})
}

func (homieClient *client) refreshId() {
	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	mac, ip, err := findMacAndIP(ifaces)
	if err != nil {
		panic(err)
	}
	id := generateHomieID(mac)
	if err != nil {
		panic(err)
	}
	homieClient.ip = ip
	homieClient.mac = mac
	homieClient.id = id
}

func (homieClient *client) onConnectHandler(client mqtt.Client) {
	self, err := os.Executable()
	if err != nil {
		homieClient.logger.Error(err)
		self = ""
	}

	homieClient.logger.Infof("connecting to mqtt broker - prefix is %s", homieClient.getDevicePrefix())
	homieClient.publishDeviceState(InitState)
	homieClient.publish("$online", "false")
	homieClient.publish("$homie", "3.0.0")
	homieClient.publish("$name", homieClient.Name())
	homieClient.publish("$mac", homieClient.Mac())
	homieClient.publish("$stats/interval", "30")
	homieClient.publish("$stats/uptime", "0")
	homieClient.publish("$localip", homieClient.Ip())
	homieClient.publish("$fw/name", homieClient.FirmwareName())
	homieClient.publish("$fw/version", "0.0.1")
	homieClient.publish("$fw/checksum", homieClient.firmwareChecksum(self))
	homieClient.publish("$implementation", "vx-go-homie")
	homieClient.subscribe("$implementation/ota/firmware/+", func(topic string, payload string) {
		checksum := strings.TrimPrefix(topic,
			fmt.Sprintf("devices/%s/$implementation/ota/firmware/", homieClient.Id()))
		homieClient.handleOTA(checksum, payload)
	})
	homieClient.publish("$implementation/ota/enabled", "true")
	nodes := make([]string, len(homieClient.Nodes()))
	i := 0
	for name, node := range homieClient.Nodes() {
		nodes[i] = name
		homieClient.logger.Debugf("publishing node %s", name)
		node.Publish()
		i++
	}
	homieClient.publish("$nodes", strings.Join(nodes, ","))
	// $online must be sent last
	homieClient.publish("$online", "true")
	homieClient.publishDeviceState(ReadyState)
	homieClient.logger.Infof("connection to mqtt broker established")
	homieClient.Connected() <- struct{}{}
}

func (homieClient *client) Start(ctx context.Context) error {
	homieClient.logger.Debug("creating mqtt client")
	homieClient.logger.Debug("using config %s", homieClient.cfgStore.Dump())
	homieClient.refreshId()
	homieClient.mqttClient = mqtt.NewClient(homieClient.getMQTTOptions())
	homieClient.bootTime = time.Now()
	homieClient.logger.Debug("connecting to mqtt server ", homieClient.Url())
	for !homieClient.mqttClient.IsConnected() {
		if token := homieClient.mqttClient.Connect(); token.Wait() && token.Error() != nil {
			fmt.Println(token.Error().Error())
			homieClient.logger.Error(token.Error())
			homieClient.logger.Warn("connection to mqtt server failed. will retry in 5 seconds")
			select {
			case <-time.After(5 * time.Second):
				homieClient.logger.Debugf("retrying")
			case <-ctx.Done():
				homieClient.logger.Error("could not connect to MQTT: we are being shutdown")
				return errors.New("could not connect to MQTT: we are being shutdown")
			}
		} else {
			homieClient.logger.Debug("connected to mqtt server")

			go homieClient.loop(ctx)
		}
	}
	return nil
}

func (homieClient *client) loop(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	homieClient.cancel = cancel
	homieClient.logger.Info("mqtt subsystem started")
	go homieClient.statsPublisher(ctx)
}

func (homieClient *client) statsPublisher(ctx context.Context) {
	ticker := time.Tick(30 * time.Second)
	for {
		select {
		case <-ticker:
			homieClient.publishStats()
			break
		case <-ctx.Done():
			return
		}
	}
}
func (homieClient *client) publishStats() {
	if homieClient.mqttClient.IsConnected() {
		infos := syscall.Sysinfo_t{}
		if syscall.Sysinfo(&infos) == nil {
			homieClient.publish("$stats/uptime", strconv.Itoa(int(infos.Uptime)))
			homieClient.publish("$stats/freeheap", strconv.Itoa(int(infos.Freeram)))
			if percent, err := cpu.Percent(0*time.Second, false); err == nil {
				homieClient.publish("$stats/cpuload", strconv.FormatFloat(percent[0], 'f', 1, 64))
			}
		}
		homieClient.publish("$stats", "uptime,freeheap,cpuload")
	}
}
func (homieClient *client) Stop() error {
	homieClient.logger.Info("stopping mqtt subsystem")
	homieClient.cancel()
	homieClient.mqttClient.Publish(homieClient.getDevicePrefix()+"$online", 1, true, "false")
	homieClient.mqttClient.Disconnect(10000)
	homieClient.logger.Info("mqtt subsystem stopped")
	return nil
}

func (homieClient *client) AddNode(name string, nodeType string) {
	homieClient.logger.Infof("adding node %s", name)
	homieClient.nodes[name] = NewNode(
		name, nodeType, homieClient.logger.WithField("node", name),
		func(path string, value string) {
			homieClient.publish(path, value)
		},
		func(topic string, callback func(topic, payload string)) {
			homieClient.subscribe(topic, callback)
		})
}

func (homieClient *client) Restart(ctx context.Context) error {
	homieClient.logger.Info("restarting mqtt subsystem")
	homieClient.Stop()
	err := homieClient.Start(ctx)
	if err == nil {
		for _, node := range homieClient.Nodes() {
			homieClient.logger.Info("restoring node ", node.Name())
			node.Publish()
		}
		for idx, callback := range homieClient.configCallbacks {
			homieClient.logger.Info("restoring publish ", idx)
			homieClient.subscribe("$implementation/config/set", func(path string, payload string) {
				callback(payload)
			})
		}
		return nil
	} else {
		homieClient.logger.Error("could not finish restart: mqtt subsystem failed to start")
		return errors.New("could not finish restart: mqtt subsystem failed to start")
	}
}
