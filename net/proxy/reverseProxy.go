/*
 * NETCAP - Traffic Analysis Framework
 * Copyright (c) 2017 Philipp Mieden <dreadl0ck [at] protonmail [dot] ch>
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package proxy

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/dreadl0ck/netcap"
	"github.com/dreadl0ck/netcap/types"

	"go.uber.org/zap"
)

// ReverseProxy represents a named reverse proxy
// that uses a custom http.Transport to export netcap audit records
type ReverseProxy struct {
	Name   string
	rp     *httputil.ReverseProxy
	writer *netcap.Writer
}

// ServeHTTP implements the http.Handler interface
func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.rp.ServeHTTP(w, r)
}

// ReverseProxyConfig represents the configuration of a single reverse proxy
// if the TLS field is set to true
// paths to the cert and key files must be specified
type ReverseProxyConfig struct {

	// Remote endpoint address
	Remote string `yaml:"remote"`

	// Local endpoint address
	Local string `yaml:"local"`

	// TLS for local endpoint
	TLS bool `yaml:"tls"`
}

// NewReverseProxy creates a ReverseProxy instance for the given target URL
// and sets the specified name
func NewReverseProxy(proxyName string, targetURL *url.URL) *ReverseProxy {

	// instantiate proxy
	proxy := &ReverseProxy{
		Name: proxyName,
		rp:   httputil.NewSingleHostReverseProxy(targetURL),
	}

	// overwrite error handler to collect the error messages
	proxy.rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// reply with BadGateway
		w.WriteHeader(http.StatusBadGateway)
		Log.Error("reverse proxy encountered an error",
			zap.String("host", r.URL.Host),
			zap.String("path", r.URL.Path),
			zap.String("method", r.Method),
			zap.String("error", err.Error()),
		)
	}

	// overwrite transport for reverse proxy
	// (needed to implement a custom roundtripper that collects metrics for us)
	proxy.rp.Transport = &NetcapTransport{

		targetURL: targetURL,
		proxyName: proxyName,
		proxy:     proxy,

		// init round tripper
		rt: &http.Transport{

			// setup DialContext
			DialContext: (&net.Dialer{
				Timeout:   time.Duration(flagDialTimeout) * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,

			Proxy:        http.ProxyFromEnvironment,
			MaxIdleConns: flagMaxIdleConns,

			// set timeouts
			IdleConnTimeout:       time.Duration(flagIdleConnTimeout) * time.Second,
			TLSHandshakeTimeout:   time.Duration(flagTLSHandshakeTimeout) * time.Second,
			ExpectContinueTimeout: 5 * time.Second,

			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: flagSkipTLSVerify,
			},
		},
	}

	proxy.writer = netcap.NewWriter("HTTP["+targetURL.Host+"]", true, true, false, "", false)
	proxy.writer.WriteHeader(types.Type_NC_HTTP, targetURL.String(), netcap.Version, false)

	return proxy
}
