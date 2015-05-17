package yetanothersmtpd

import (
	"net/mail"
	"strings"
)

func parseAddress(src string) (string, error) {
	if !strings.HasPrefix(src, "<") || !strings.HasSuffix(src, ">") {
		return "", ErrMalformedEmail
	}
	addr, err := mail.ParseAddress(src)
	if err != nil {
		return "", ErrMalformedEmail
	}
	return addr.Address, nil
}
