package network

import (
	"context"
	"net"
	"net/http"
	"time"

	utls "github.com/refraction-networking/utls"
)

// uTLSDialer создаёт кастомный DialTLSContext с JA3 fingerprint spoofing.
// Использует uTLS вместо стандартного crypto/tls для имитации реальных браузеров.
// Это критично для обхода Cloudflare и других WAF, которые анализируют TLS handshake.
type uTLSDialer struct {
	netDialer *net.Dialer
}

// newUTLSDialer возвращает dialer с Chrome-подобным JA3 fingerprint.
func newUTLSDialer(timeout time.Duration) *uTLSDialer {
	return &uTLSDialer{
		netDialer: &net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		},
	}
}

// DialContext реализует контекстный dial с uTLS.
// Использует HelloChrome_Auto для максимальной совместимости.
func (d *uTLSDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	plainConn, err := d.netDialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	// uTLS использует свой Config, совместимый с crypto/tls
	cfg := &utls.Config{
		InsecureSkipVerify: false,
		MinVersion:         utls.VersionTLS12,
	}

	uconn := utls.UClient(plainConn, cfg, utls.HelloChrome_Auto)
	if err := uconn.HandshakeContext(ctx); err != nil {
		plainConn.Close()
		return nil, err
	}
	return uconn, nil
}

// EnableUTLS добавляет uTLS dial в transport.
// Если useUTLS == false, возвращает стандартный DialContext.
func EnableUTLS(tr *http.Transport, useUTLS bool) {
	if !useUTLS {
		return
	}
	dialer := newUTLSDialer(5 * time.Second)
	tr.DialTLSContext = dialer.DialContext
}
