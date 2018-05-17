package api

type HelloSayHelloReq struct {
	Str string `json:"str"`
}

type HelloSayHelloRsp struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
	Str   string `json:"str"`
}
