package homie

import (
	"errors"
	"net"
	"strings"
)

func findMacAndIP(ifs []net.Interface) (string, string, error) {
	for _, v := range ifs {
		if v.Flags&net.FlagLoopback != net.FlagLoopback && v.Flags&net.FlagUp == net.FlagUp {
			h := v.HardwareAddr.String()
			if len(h) == 0 {
				continue
			} else {
				addresses, _ := v.Addrs()
				if len(addresses) > 0 {
					ip := strings.Split(addresses[0].String(), "/")[0]
					return h, ip, nil
				}
			}
		}
	}
	return "", "", errors.New("could not find a valid network interface")

}

func generateHomieID(mac string) string {
	return strings.Replace(mac, ":", "", -1)
}

func (homieClient *client) getDevicePrefix() string {
	return homieClient.Prefix() + homieClient.Id() + "/"
}
