package transport

type Server interface {
	Serve(chan struct{}) error
	Close()
}
