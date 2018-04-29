package homie

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"strings"
)

type OTASession struct {
	checksum string
	buff     string
}

func (homieClient *client) firmwareChecksum(path string) string {
	fd, err := os.Open(path)
	if err != nil {
		homieClient.logger.Error(err)
		return ""
	}
	defer fd.Close()
	hash := md5.New()
	if _, err := io.Copy(hash, fd); err != nil {
		homieClient.logger.Error(err)
		return ""
	}

	return fmt.Sprintf("%x", hash.Sum(nil))
}
func (homieClient *client) handleOTA(checksum string, firmware string) {
	homieClient.logger.Info(("OTA started"))

	hash := md5.New()
	if _, err := io.Copy(hash, strings.NewReader(firmware)); err != nil {
		homieClient.publish("$implementation/ota/status", "400 BAD_CHECKSUM")
		homieClient.logger.Errorf("aborting OTA : failed to compute checksum: %v", err)
		return
	}
	receivedChecksum := fmt.Sprintf("%x", hash.Sum(nil))
	if receivedChecksum != checksum {
		homieClient.publish("$implementation/ota/status", "400 BAD_CHECKSUM")
		homieClient.logger.Errorf("aborting OTA : checksum mismatch: received %q, wanted %q", receivedChecksum, checksum)
		return
	}
	self, err := os.Executable()
	if err != nil {
		homieClient.publish("$implementation/ota/status", "500")
		homieClient.logger.Errorf("aborting OTA : %v", err)
		return
	}
	fd, err := os.Create(self + "-ota")
	if err != nil {
		homieClient.publish("$implementation/ota/status", "500")
		homieClient.logger.Errorf("aborting OTA : %v", err)
		return
	}
	buff := make([]byte, 4096)
	reader := strings.NewReader(firmware)
	count := 0
	total := len(firmware)
	for {
		if count%(total/20) == 0 {
			homieClient.publish("$implementation/ota/status", fmt.Sprintf("206 %d/%d", count, total))
		}
		_, err := reader.Read(buff)
		if err == io.EOF {
			break
		}
		if err != nil {
			homieClient.publish("$implementation/ota/status", "500")
			homieClient.logger.Errorf("aborting OTA : %v", err)
			return
		}
		n, err := fd.Write(buff)
		if err != nil {
			homieClient.publish("$implementation/ota/status", "500")
			homieClient.logger.Errorf("aborting OTA : %v", err)
			return
		}
		count += n
	}
	homieClient.publish("$implementation/ota/status", fmt.Sprintf("206 %d/%d", total, total))
	fd.Close()
	err = os.Rename(self+"-ota", self)
	if err != nil {
		homieClient.publish("$implementation/ota/status", "500")
		homieClient.logger.Errorf("aborting OTA : %v", err)
		return
	}
	homieClient.publish("$implementation/ota/status", fmt.Sprintf("200"))
	homieClient.logger.Info(("OTA succeeded, restart is required"))
}
