package api

type DispatchGreetReq struct {
	Str string `json:"str"`
}

type DispatchGreetRsp struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
	Str   string `json:"str"`
}
