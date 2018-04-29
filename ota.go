package homie

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
)

func (homieClient *client) firmwareChecksum() string {
	self, err := os.Executable()
	if err != nil {
		homieClient.logger.Error(err)
		return ""
	}
	fd, err := os.Open(self)
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
