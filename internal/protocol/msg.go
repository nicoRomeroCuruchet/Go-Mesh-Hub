package protocol

import (
	"encoding/binary"
	"errors"
	"net"
)

const (
	MsgData      byte = 0x01
	MsgPeerInfo  byte = 0x02
	MsgHolePunch byte = 0x03
)

// EncodePeerInfo packs virtualIP + real UDP address into 10 bytes:
// virtualIP(4) + realIP(4) + realPort(2)
func EncodePeerInfo(virtualIP net.IP, addr *net.UDPAddr) []byte {
	buf := make([]byte, 10)
	copy(buf[0:4], virtualIP.To4())
	copy(buf[4:8], addr.IP.To4())
	binary.BigEndian.PutUint16(buf[8:10], uint16(addr.Port))
	return buf
}

// DecodePeerInfo unpacks a 10-byte PEER_INFO payload
func DecodePeerInfo(data []byte) (virtualIP net.IP, addr *net.UDPAddr, err error) {
	if len(data) < 10 {
		return nil, nil, errors.New("peer info payload too short")
	}
	virtualIP = make(net.IP, 4)
	copy(virtualIP, data[0:4])
	realIP := make(net.IP, 4)
	copy(realIP, data[4:8])
	port := int(binary.BigEndian.Uint16(data[8:10]))
	return virtualIP, &net.UDPAddr{IP: realIP, Port: port}, nil
}

// EncodeHolePunch packs the sender's virtual IP into 4 bytes
func EncodeHolePunch(virtualIP net.IP) []byte {
	buf := make([]byte, 4)
	copy(buf, virtualIP.To4())
	return buf
}

// DecodeHolePunch extracts the virtual IP from a HOLE_PUNCH payload
func DecodeHolePunch(data []byte) (net.IP, error) {
	if len(data) < 4 {
		return nil, errors.New("hole punch payload too short")
	}
	ip := make(net.IP, 4)
	copy(ip, data[0:4])
	return ip, nil
}
