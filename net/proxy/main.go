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
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// a list of all reverse proxies
// used to close all files handles on exit via OS signals
var proxies []*ReverseProxy

func GetCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "HTTP Reverse Proxy",
		Run: func(cmd *cobra.Command, args []string) {

			// check if flags have been used to configure a single instance proxy
			if flagLocal == "" || flagRemote == "" {
				// parse config file
				var errParseConfig error
				c, errParseConfig = ParseConfiguration(flagConfig)
				if errParseConfig != nil {
					log.Fatal("failed to parse config: ", errParseConfig)
				}
			} else {
				// setup single proxy instance
				c = &Config{
					Proxies: map[string]ReverseProxyConfig{
						"customproxy": ReverseProxyConfig{
							Remote: flagRemote,
							Local:  flagLocal,
						},
					},
				}
			}

			// handle OS signals
			handleSignals()

			// print configuration
			fmt.Println("Configuration:")
			c.Dump(os.Stdout)

			// configure logger
			ConfigureLogger(flagDebug, filepath.Join(c.Logdir, LogFileName))

			// synchronize the logger on exit
			defer Log.Sync()

			Log.Info("setup complete",
				zap.String("logfile", LogFileName),
				zap.String("config", flagConfig),
			)

			// iterate over proxies from config
			for name, p := range c.Proxies {

				// copy variables to avoid capturing them
				// when dispatching a goroutine
				var (
					proxyName = name
					local     = p.Local
					remote    = p.Remote
					tls       = p.TLS
				)

				// spawn a goroutine for each proxy
				go func() {

					Log.Info("initializing proxy",
						zap.String("local", local),
						zap.String("remote", remote),
						zap.String("proxyName", proxyName),
					)

					// parse remote URL
					targetURL, errURL := url.Parse(remote)
					if errURL != nil {
						panic(errURL)
					}

					// instantiate proxy
					p := NewReverseProxy(proxyName, targetURL)
					proxies = append(proxies, p)
					if tls {

						// check if key and cert file have been specified
						if c.CertFile == "" || c.KeyFile == "" {
							log.Fatal(proxyName, " configured to use TLS for local endpoint, but no missing cert and key in config.")
						}

						// start serving HTTPS
						err := http.ListenAndServeTLS(local, c.CertFile, c.KeyFile, p)
						if err != nil {
							log.Fatal(proxyName, " failed. error: ", err)
						}
					} else {
						// start serving HTTP
						err := http.ListenAndServe(local, p)
						if err != nil {
							log.Fatal(proxyName, " failed. error: ", err)
						}
					}
				}()
			}

			// wait until the end of time
			<-make(chan bool)
		},
	}

	cmd.Flags().BoolVarP(&flagDebug, "debug", "d", false, "toggle debug mode")
	cmd.Flags().BoolVarP(&flagTrace, "trace", "t", false, "trace HTTP requests to retrieve additional information")
	cmd.Flags().BoolVarP(&flagDump, "dump", "D", false, "dumps audit record as JSON to stdout")
	cmd.Flags().BoolVarP(&flagSkipTLSVerify, "skipTlsVerify", "", false, "skip TLS verification")
	cmd.Flags().BoolVarP(&flagDumpFormatted, "format", "f", true, "format when dumping JSON")

	cmd.Flags().StringVarP(&flagLocal, "local", "l", "", "set local endpoint")
	cmd.Flags().StringVarP(&flagConfig, "config", "c", "net.proxy-config.yml", "set config file path")
	cmd.Flags().StringVarP(&flagRemote, "remote", "r", "", "set remote endpoint")

	cmd.Flags().IntVarP(&flagDialTimeout, "dialTimeout", "T", 30, "seconds until dialing to the backend times out")
	cmd.Flags().IntVarP(&flagIdleConnTimeout, "idleConnTimeout", "i", 90, "seconds until a connection times out")
	cmd.Flags().IntVarP(&flagTLSHandshakeTimeout, "tlsTimeout", "", 15, "seconds until a TLS handshake times out")
	cmd.Flags().IntVarP(&flagMaxIdleConns, "maxIdle", "m", 120, "maximum number of idle connections")

	return cmd
}
