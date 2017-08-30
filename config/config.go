package config

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"
)

type Store interface {
	Get() *Format
	Stop()
	Save()
}

type store struct {
	Path   string
	logger *logrus.Entry
	Data   *Format
	Db     *bolt.DB
	cancel context.CancelFunc
}

func NewStore(ctx context.Context, path string, logger *logrus.Entry) Store {
	db, err := bolt.Open(path+"config.Db", 0600, nil)
	if err != nil {
		panic(err)
	}
	s := &store{Path: path, Db: db, logger: logger}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.LoadPersisted()
	go s.start(ctx)
	return s
}

/*
 {
   "mqtt": {
     "host": "192.0.2.1",
     "port": 1883,
     "ssl": true,
     "ssl_auth": true
   },
   "homie": {
     "name:" "weatherController"
    }
 }
*/

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
	Prefix     string    `json:"prefix,omitempty"`
	Host       string    `json:"host,omitempty"`
	Port       int       `json:"port,omitempty"`
	Ssl        bool      `json:"ssl,omitempty"`
	Ssl_Config TLSFormat `json:"ssl_config,omitempty"`
}
type Format struct {
	Mqtt        MQTTFormat  `json:"mqtt,omitempty"`
	Homie       HomieFormat `json:"homie,omitempty"`
	Initialized bool
}

func (s *store) start(ctx context.Context) {
	select {
	case <-ctx.Done():
		s.Db.Close()
	}
}
func (s *store) LoadDefaults() {
	s.logger.Debug("loading default configuration")
	s.Data = &Format{
		Initialized: true,
		Mqtt: MQTTFormat{
			Prefix: "",
			Host:   "iot.eclipse.org",
			Port:   1883,
			Ssl:    false,
			Ssl_Config: TLSFormat{
				Privkey:    "",
				CA:         "",
				ClientCert: "",
			},
		},
		Homie: HomieFormat{
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

func (s *store) Save() {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Unknown error occured when saving config")
			s.logger.Error(r)
		}
	}()
	s.logger.Debug("updating saved configuration")
	s.logger.Debug("opening database")
	s.logger.Debug("aquiring r/w transaction")
	s.Db.Update(func(tx *bolt.Tx) error {
		s.logger.Debug("transaction aquired")
		b := tx.Bucket([]byte("config"))
		if b == nil {
			var err error
			b, err = tx.CreateBucket([]byte("config"))
			if err != nil {
				return err
			}
		}
		buf, err := json.Marshal(s.Data)
		if err != nil {
			return err
		}
		s.logger.Debug("updating Data")
		return b.Put([]byte("store"), buf)
	})
	s.logger.Debug("configuration updated")
}

func (s *store) LoadPersisted() {
	if s.Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("config"))
		if b != nil {
			v := b.Get([]byte("store"))
			json.Unmarshal(v, &s.Data)
			return nil
		} else {
			s.logger.Warn("no persisted configuration found")
			return errors.New("bucket 'config' not found")
		}
	}) != nil {
		s.LoadDefaults()
	}

}
func (s *store) Get() *Format {
	return s.Data
}
func (s *store) Stop() {
	s.cancel()
}
