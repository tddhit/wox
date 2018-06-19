package transport

type Server interface {
	Serve() error
	Close()
}
