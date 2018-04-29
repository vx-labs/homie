package homie

import (
	"context"
	"strconv"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/sirupsen/logrus"
	"github.com/vx-labs/homie/config"
)

type publishFunc func(property string, value string)
type subscribeFunc func(topic string, callback func(topic, payload string))

type Client interface {
	EnableTLS()
	SetCustomTLSConfiguration(ssl_ca string, ssl_cert string, ssl_key string)
	SetCredentials(user, password string)
	Start(ctx context.Context) error
	Connected() chan struct{}
	Disconnected() chan struct{}
	Restart(ctx context.Context) error
	Name() string
	Id() string
	Url() string
	Ip() string
	Prefix() string
	Mac() string
	Stop() error
	FirmwareName() string
	AddConfigCallback(func(config string))
	AddNode(name string, nodeType string)
	Nodes() map[string]Node
	Reconfigure(ctx context.Context, prefix string, host string, port int, mqttPrefix string, ssl bool, sslAuth *config.TLSFormat, deviceName string)
}
type SettableProperty struct {
	Name     string
	Callback func(payload string)
}

type stateMessage struct {
	subtopic string
	payload  string
}
type subscribeMessage struct {
	subtopic string
	callback func(path string, payload string)
}
type unsubscribeMessage struct {
	subtopic string
}

type client struct {
	id              string
	wg              sync.WaitGroup
	logger          *logrus.Entry
	cfgStore        config.Store
	ip              string
	mac             string
	firmwareName    string
	cancel          context.CancelFunc
	publishChan     chan stateMessage
	subscribeChan   chan subscribeMessage
	unsubscribeChan chan unsubscribeMessage
	bootTime        time.Time
	mqttClient      mqtt.Client
	nodes           map[string]Node
	configCallbacks []func(config string)
	connected       chan struct{}
	disconnected    chan struct{}
	otaSessions     map[string]OTASession
	restartHandler  func()
}

func (homieClient *client) Id() string {
	return homieClient.id
}

func (homieClient *client) Prefix() string {
	return homieClient.cfgStore.Get().Homie.Prefix
}

func (homieClient *client) Url() string {
	url := homieClient.cfgStore.Get().Mqtt.Host + ":" + strconv.Itoa(homieClient.cfgStore.Get().Mqtt.Port)
	if homieClient.cfgStore.Get().Mqtt.Ssl {
		url = "ssl://" + url
	} else {
		url = "tcp://" + url
	}
	url = url + homieClient.cfgStore.Get().Mqtt.Prefix
	return url
}
func (homieClient *client) Mac() string {
	return homieClient.mac
}
func (homieClient *client) Ip() string {
	return homieClient.ip
}
func (homieClient *client) Name() string {
	return homieClient.cfgStore.Get().Homie.Name
}

func (homieClient *client) FirmwareName() string {
	return homieClient.firmwareName
}
func (homieClient *client) Nodes() map[string]Node {
	return homieClient.nodes
}

func (homieClient *client) AddConfigCallback(callback func(config string)) {
	homieClient.subscribe("$implementation/config/set", func(path string, payload string) {
		callback(payload)
	})
	homieClient.configCallbacks = append(homieClient.configCallbacks, callback)
}

func (homieClient *client) Reconfigure(ctx context.Context, prefix string, host string, port int, mqttPrefix string, ssl bool, sslConfig *config.TLSFormat, deviceName string) {
	cfg := homieClient.cfgStore.Get()
	cfg.Homie.Name = deviceName
	cfg.Mqtt.Prefix = mqttPrefix
	cfg.Homie.Prefix = prefix
	cfg.Mqtt.Host = host
	cfg.Mqtt.Port = port
	cfg.Mqtt.Ssl = ssl
	cfg.Mqtt.Ssl_Config = sslConfig
	homieClient.logger.Info("configuration changed: restarting")
	homieClient.Restart(ctx)
}
