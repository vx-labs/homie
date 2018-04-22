package config

import (
	"context"
	"encoding/json"

	"github.com/sirupsen/logrus"
)

type Store interface {
	Get() *Format
	Dump() string
	Stop()
}

type store struct {
	Path   string
	logger *logrus.Entry
	Data   *Format
	cancel context.CancelFunc
}

func NewStore(ctx context.Context, path string, logger *logrus.Entry) Store {
	s := &store{logger: logger}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.LoadPersisted()
	go s.start(ctx)
	return s
}

type HomieFormat struct {
	Name   string `json:"name,omitempty"`
	Prefix string `json:"prefix"`
}
type TLSFormat struct {
	CA         string `json:"ca"`
	ClientCert string `json:"client_cert"`
	Privkey    string `json:"privkey"`
}
type MQTTFormat struct {
	Prefix     string     `json:"prefix,omitempty"`
	Host       string     `json:"host,omitempty"`
	Port       int        `json:"port,omitempty"`
	Ssl        bool       `json:"ssl,omitempty"`
	Ssl_Config *TLSFormat `json:"ssl_config,omitempty"`
	Username   string     `json:"username,omitempty"`
	Password   string     `json:"username,omitempty"`
}
type Format struct {
	Mqtt        *MQTTFormat  `json:"mqtt,omitempty"`
	Homie       *HomieFormat `json:"homie,omitempty"`
	Initialized bool
}

func (s *store) start(ctx context.Context) {
	select {
	case <-ctx.Done():
	}
}
func (s *store) LoadDefaults() {
	s.logger.Debug("loading default configuration")
	s.Data = &Format{
		Initialized: false,
		Mqtt: &MQTTFormat{
			Prefix: "",
			Host:   "iot.eclipse.org",
			Port:   1883,
			Ssl:    false,
			Ssl_Config: &TLSFormat{
				Privkey:    "",
				CA:         "",
				ClientCert: "",
			},
		},
		Homie: &HomieFormat{
			Name:   "go-homie",
			Prefix: "devices/",
		},
	}
}

func (s *store) MergeJSONString(payload string) {
	if err := json.Unmarshal([]byte(payload), &s.Data); err != nil {
		s.logger.Error(err)
	}
}

func (s *store) Dump() string {
	buf, _ := json.Marshal(s.Data)
	return string(buf)
}

func (s *store) LoadPersisted() {
	s.LoadDefaults()
}
func (s *store) Get() *Format {
	return s.Data
}
func (s *store) Stop() {
	s.cancel()
}
