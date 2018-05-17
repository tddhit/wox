package api

type HiSayHiReq struct {
	Str string `json:"str"`
}

type HiSayHiRsp struct {
	Code  int    `json:"code"`
	Error string `json:"error"`
	Str   string `json:"str"`
}
