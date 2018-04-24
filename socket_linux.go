package wox

import (
	"net"

	"golang.org/x/sys/unix"

	"github.com/tddhit/tools/log"
)

func Listen(tcpAddr *net.TCPAddr) (fd int, err error) {
	sockaddr := &unix.SockaddrInet4{Port: tcpAddr.Port}
	if ipv4 := tcpAddr.IP.To4(); ipv4 != nil {
		addr := [4]byte{ipv4[0], ipv4[1], ipv4[2], ipv4[3]}
		sockaddr.Addr = addr
	}
	fd, err = unix.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if err != nil {
		log.Error(err)
		return
	}
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
	if err != nil {
		log.Error(err)
		return
	}
	err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	if err != nil {
		log.Error(err)
		return
	}
	err = unix.Bind(fd, sockaddr)
	if err != nil {
		log.Error(err)
		return
	}
	err = unix.Listen(fd, 128)
	if err != nil {
		log.Error(err)
		return
	}
	return
}
