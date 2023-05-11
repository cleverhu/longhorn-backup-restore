package util

import "github.com/parnurzeal/gorequest"

func NewRequestClient() *gorequest.SuperAgent {
	request := gorequest.New()
	return request
}
