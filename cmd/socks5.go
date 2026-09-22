package cmd

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

// Constantes do protocolo SOCKS5 (RFC 1928)
const (
	socks5Version = 0x05

	socks5AuthNone         = 0x00
	socks5AuthNoAcceptable = 0xFF

	socks5CmdConnect = 0x01

	socks5AtypIPv4   = 0x01
	socks5AtypDomain = 0x03
	socks5AtypIPv6   = 0x04

	socks5RepSuccess          = 0x00
	socks5RepGeneralFailure   = 0x01
	socks5RepCmdNotSupported  = 0x07
	socks5RepAtypNotSupported = 0x08
)

// socks5DialFunc abre a conexão com o destino solicitado pelo cliente SOCKS
type socks5DialFunc func(network, address string) (net.Conn, error)

// socks5Handshake executa a negociação SOCKS5 com o cliente e conecta ao destino usando dial.
// Suporta apenas o comando CONNECT sem autenticação. Nomes de domínio são repassados
// sem resolução local, permitindo que o DNS seja resolvido do lado remoto do tunnel.
// Retorna o endereço de destino e a conexão remota já estabelecida.
func socks5Handshake(client net.Conn, dial socks5DialFunc) (string, net.Conn, error) {
	// Saudação: VER | NMETHODS | METHODS
	header := make([]byte, 2)
	if _, err := io.ReadFull(client, header); err != nil {
		return "", nil, fmt.Errorf("erro ao ler saudação SOCKS: %w", err)
	}
	if header[0] != socks5Version {
		return "", nil, fmt.Errorf("versão SOCKS não suportada: %d", header[0])
	}

	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(client, methods); err != nil {
		return "", nil, fmt.Errorf("erro ao ler métodos de autenticação SOCKS: %w", err)
	}

	noAuth := false
	for _, m := range methods {
		if m == socks5AuthNone {
			noAuth = true
			break
		}
	}
	if !noAuth {
		client.Write([]byte{socks5Version, socks5AuthNoAcceptable})
		return "", nil, fmt.Errorf("cliente SOCKS não aceita conexão sem autenticação")
	}
	if _, err := client.Write([]byte{socks5Version, socks5AuthNone}); err != nil {
		return "", nil, fmt.Errorf("erro ao responder saudação SOCKS: %w", err)
	}

	// Requisição: VER | CMD | RSV | ATYP | DST.ADDR | DST.PORT
	req := make([]byte, 4)
	if _, err := io.ReadFull(client, req); err != nil {
		return "", nil, fmt.Errorf("erro ao ler requisição SOCKS: %w", err)
	}
	if req[0] != socks5Version {
		return "", nil, fmt.Errorf("versão SOCKS inválida na requisição: %d", req[0])
	}

	var host string
	switch req[3] {
	case socks5AtypIPv4:
		addr := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(client, addr); err != nil {
			return "", nil, fmt.Errorf("erro ao ler endereço IPv4: %w", err)
		}
		host = net.IP(addr).String()
	case socks5AtypIPv6:
		addr := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(client, addr); err != nil {
			return "", nil, fmt.Errorf("erro ao ler endereço IPv6: %w", err)
		}
		host = net.IP(addr).String()
	case socks5AtypDomain:
		length := make([]byte, 1)
		if _, err := io.ReadFull(client, length); err != nil {
			return "", nil, fmt.Errorf("erro ao ler tamanho do domínio: %w", err)
		}
		domain := make([]byte, int(length[0]))
		if _, err := io.ReadFull(client, domain); err != nil {
			return "", nil, fmt.Errorf("erro ao ler domínio: %w", err)
		}
		host = string(domain)
	default:
		socks5Reply(client, socks5RepAtypNotSupported)
		return "", nil, fmt.Errorf("tipo de endereço SOCKS não suportado: %d", req[3])
	}

	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(client, portBytes); err != nil {
		return "", nil, fmt.Errorf("erro ao ler porta de destino: %w", err)
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes))))

	if req[1] != socks5CmdConnect {
		socks5Reply(client, socks5RepCmdNotSupported)
		return target, nil, fmt.Errorf("comando SOCKS não suportado: %d (apenas CONNECT)", req[1])
	}

	remote, err := dial("tcp", target)
	if err != nil {
		socks5Reply(client, socks5RepGeneralFailure)
		return target, nil, err
	}

	if err := socks5Reply(client, socks5RepSuccess); err != nil {
		remote.Close()
		return target, nil, fmt.Errorf("erro ao responder requisição SOCKS: %w", err)
	}

	return target, remote, nil
}

// socks5Reply envia a resposta da requisição com endereço de bind zerado
func socks5Reply(client net.Conn, rep byte) error {
	_, err := client.Write([]byte{socks5Version, rep, 0x00, socks5AtypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}
