package ssh

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Config struct {
	User       string
	Host       string
	Port       int
	PrivateKey []byte
}

func (conf *Config) EncodeWithDSN(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("cannot parse DSN during encode: %w", err)
	}

	values := parsed.Query()
	values.Add("ssh_private_key", string(conf.PrivateKey))

	parsed.RawQuery = values.Encode()
	updatedBase := parsed.String()
	split := strings.Split(updatedBase, "://")

	if len(split) != 2 {
		return "", fmt.Errorf("invalid DSN format during encode: splitting by :// gives unexpected results")
	}

	return fmt.Sprintf(
		"%s://%s@%s:%d/%s", split[0], conf.User, conf.Host, conf.Port, split[1]), nil
}

func (conf *Config) DecodeFromDSN(encodedDSN string) (dsn string, err error) {
	parsed, err := url.Parse(encodedDSN)
	if err != nil {
		return "", fmt.Errorf("cannot parse DSN during decode: %w", err)
	}

	conf.User = parsed.User.Username()
	conf.Host = parsed.Hostname()
	conf.Port, _ = strconv.Atoi(parsed.Port())
	conf.PrivateKey = []byte(parsed.Query().Get("ssh_private_key"))

	values := parsed.Query()
	values.Del("ssh_private_key")

	parsed.RawQuery = values.Encode()

	// remove the middle information of scheme://ssh_user:ssh_password@ssh_host:ssh_port/
	split := strings.Split(parsed.String(), "://")
	if len(split) != 2 {
		return "", fmt.Errorf("invalid DSN format during decode: splitting by :// gives unexpected results")
	}

	idx := strings.Index(split[1], "/")
	if idx == -1 {
		return "", errors.New("invalid DSN format: missing / after ssh_port")
	}

	return fmt.Sprintf("%s://%s", split[0], split[1][idx+1:]), nil
}
