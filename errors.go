package wox

import "errors"

var (
	Err400Rsp = errors.New(`{"code":400,"error": "Invalid Request"}`)
	Err500Rsp = errors.New(`{"code":500,"error": "Internal Server Error"}`)
	Err503Rsp = errors.New(`{"code":503,"error": "Service Unavailable"}`)
)

var (
	errUnsupportedMethod      = errors.New("Unsupported Method")
	errUnsupportedContentType = errors.New("Unsupported ContentType")
	errUnavailableUpstream    = errors.New("Unavailable Upstream")
)
