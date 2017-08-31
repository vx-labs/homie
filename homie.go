package homie

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/sirupsen/logrus"
	"github.com/vx-labs/homie/config"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"time"
)

func NewClient(ctx context.Context, prefix string, server string, port int, mqttPrefix string, ssl bool, ssl_ca string, ssl_cert string, ssl_key string, deviceName string, firmwareName string, logger *logrus.Entry) Client {
	store := config.NewStore(ctx, "/tmp/", logger)
	cfg := store.Get()
	if !cfg.Initialized {
		cfg.Mqtt.Host = server
		cfg.Mqtt.Port = port
		cfg.Mqtt.Ssl = ssl
		cfg.Mqtt.Ssl_Config.CA = ssl_ca
		cfg.Mqtt.Ssl_Config.ClientCert = ssl_cert
		cfg.Mqtt.Ssl_Config.Privkey = ssl_key
		cfg.Mqtt.Prefix = mqttPrefix

		cfg.Homie.Prefix = prefix
		cfg.Homie.Name = deviceName
		cfg.Initialized = true
		store.Save()
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
	}

}
func (homieClient *client) getMQTTOptions() *mqtt.ClientOptions {
	o := mqtt.NewClientOptions()
	o.AddBroker(homieClient.Url())
	o.SetClientID(homieClient.Id())
	o.SetWill(homieClient.getDevicePrefix()+"$online", "false", 1, true)
	o.SetKeepAlive(10 * time.Second)
	o.SetOnConnectHandler(homieClient.onConnectHandler)
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

func (homieClient *client) publish(subtopic string, payload string) {
	homieClient.publishChan <- stateMessage{subtopic: subtopic, payload: payload}
}

func (homieClient *client) unsubscribe(subtopic string) {
	homieClient.unsubscribeChan <- unsubscribeMessage{subtopic: subtopic}
}

func (homieClient *client) subscribe(subtopic string, callback func(path string, payload string)) {
	homieClient.subscribeChan <- subscribeMessage{subtopic: subtopic, callback: callback}
}

func (homieClient *client) onConnectHandler(client mqtt.Client) {
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
	go homieClient.loop()

	homieClient.publish("$homie", "2.1.0")
	homieClient.publish("$name", homieClient.Name())
	homieClient.publish("$mac", homieClient.Mac())
	homieClient.publish("$stats/interval", "10")
	homieClient.publish("$localip", homieClient.Ip())
	homieClient.publish("$fw/Name", homieClient.FirmwareName())
	homieClient.publish("$fw/version", "0.0.1")
	homieClient.publish("$implementation", "vx-go-homie")

	nodes := make([]string, len(homieClient.Nodes()))
	i := 0
	for name, node := range homieClient.Nodes() {
		nodes[i] = name
		homieClient.logger.Debugf("publishing node %s", name)
		node.Publish()
		i += 1
	}
	homieClient.publish("nodes", strings.Join(nodes, ","))
	// $online must be sent last
	homieClient.publish("$online", "true")
	go homieClient.ReadyCallback()
}

func (homieClient *client) Start(cb func()) error {
	homieClient.ReadyCallback = cb
	tries := 0
	homieClient.logger.Debug("creating mqtt client")
	homieClient.logger.Debug("using config %s", homieClient.cfgStore.Dump())
	homieClient.mqttClient = mqtt.NewClient(homieClient.getMQTTOptions())
	homieClient.bootTime = time.Now()
	homieClient.logger.Debug("connecting to mqtt server ", homieClient.Url())
	for !homieClient.mqttClient.IsConnected() && tries < 10 {
		if token := homieClient.mqttClient.Connect(); token.Wait() && token.Error() != nil {
			fmt.Println(token.Error().Error())
			homieClient.logger.Error(token.Error())
			homieClient.logger.Warn("connection to mqtt server failed. will retry in 5 seconds")
			select {
			case <-time.After(5 * time.Second):
				tries += 1
			case <-homieClient.stopChan:
				homieClient.logger.Error("could not connect to MQTT: we are being shutdown")
				homieClient.stopStatusChan <- true
				return errors.New("could not connect to MQTT: we are being shutdown")
			}
		} else {
			homieClient.logger.Debug("connected to mqtt server")
		}
	}
	if tries >= 10 {
		return errors.New("could not connect to MQTT at " + homieClient.Url())
	} else {
		return nil
	}

}

func (homieClient *client) loop() {
	run := true
	homieClient.stopChan = make(chan bool, 1)
	homieClient.stopStatusChan = make(chan bool, 1)
	homieClient.logger.Info("mqtt subsystem started")
	for run {
		select {
		case msg := <-homieClient.publishChan:
			topic := homieClient.getDevicePrefix() + msg.subtopic
			homieClient.logger.Debugf("%s <- %s", topic, msg.payload)
			homieClient.mqttClient.Publish(topic, 1, true, msg.payload)
			break
		case msg := <-homieClient.unsubscribeChan:
			topic := homieClient.getDevicePrefix() + msg.subtopic
			homieClient.mqttClient.Unsubscribe(topic)
			break
		case msg := <-homieClient.subscribeChan:
			topic := homieClient.getDevicePrefix() + msg.subtopic
			homieClient.mqttClient.Subscribe(topic, 1, func(mqttClient mqtt.Client, mqttMessage mqtt.Message) {
				msg.callback(mqttMessage.Topic(), string(mqttMessage.Payload()))
			})
			break
		case <-homieClient.stopChan:
			run = false
			break
		case <-time.After(10 * time.Second):
			homieClient.publishStats()
			break
		}
	}
	homieClient.mqttClient.Publish(homieClient.getDevicePrefix()+"$online", 1, true, "false")
	homieClient.mqttClient.Disconnect(1000)
	homieClient.stopStatusChan <- true
}

func (homieClient *client) publishStats() {
	homieClient.publish("$stats/uptime", strconv.Itoa(int(time.Since(homieClient.bootTime).Seconds())))
}
func (homieClient *client) Stop() error {
	homieClient.logger.Info("stopping mqtt subsystem")
	homieClient.stopChan <- true
	for {
		select {
		case <-homieClient.stopStatusChan:
			homieClient.logger.Info("mqtt subsystem stopped")
			return nil
			break
		}
	}
}

func (homieClient *client) AddNode(name string, nodeType string) {
	homieClient.logger.Infof("adding node %s", name)
	homieClient.nodes[name] = NewNode(
		name, nodeType, homieClient.logger.WithField("node", name),
		func(path string, value string) {
			homieClient.publish(path, value)
		})
}

func (homieClient *client) Restart() error {
	homieClient.logger.Info("restarting mqtt subsystem")
	homieClient.Stop()
	err := homieClient.Start(homieClient.ReadyCallback)
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
