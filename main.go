package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

type args struct {
	listenAddr string
	// Static site asset location
	staticBasePath string
	staticAssetDir string

	// Basic Auth require asset location
	basicAuthBasePath  string
	basicAuthAssertDir string
	basicAuthUsername  string
	basicAuthPassword  string

	// Share links, map a random/opaque url token to a fixed path
	shareBasePath string
	shares        []string // <token>=<path>
	shareGens     []string // <path>, token generated at startup

	allowOrigins     []string
	allowMethods     []string
	allowHeaders     []string
	exposeHeaders    []string
	allowCredentials bool
}

func defaultArgs() *args {
	return &args{
		listenAddr:     ":9112",
		staticBasePath: "/_static/public",
		staticAssetDir: "static",

		basicAuthBasePath:  "/_static/auth",
		basicAuthAssertDir: "statica",
		basicAuthUsername:  "alice",
		basicAuthPassword:  "secret",

		shareBasePath: "/s",

		allowOrigins:     []string{"*"},
		allowMethods:     []string{"*"},
		allowHeaders:     []string{"*"},
		exposeHeaders:    []string{"*"},
		allowCredentials: true,
	}
}

func cmd() *cobra.Command {
	a := defaultArgs()

	cmd := &cobra.Command{
		Use:   "s4",
		Short: "Simple Static Site Service",
		Long:  "Simple static site service serve static assets as a site",
		Run: func(cmd *cobra.Command, args []string) {
			runServer(a)
		},
	}

	cmd.Flags().StringVar(&a.listenAddr, "address", a.listenAddr, "the address which server will serve at")
	cmd.Flags().StringVar(&a.staticBasePath, "base-path", a.staticBasePath, "the http base path to serve static assets")
	cmd.Flags().StringVar(&a.staticAssetDir, "asset-dir", a.staticAssetDir, "the static site asset dir")
	cmd.Flags().StringVar(&a.basicAuthBasePath, "base-auth-path", a.basicAuthBasePath, "the http base path to serve basic auth enabled static assets")
	cmd.Flags().StringVar(&a.basicAuthAssertDir, "auth-asset-dir", a.basicAuthAssertDir, "the basic auth enabled static site asset dir")
	cmd.Flags().StringVar(&a.basicAuthUsername, "auth-user", a.basicAuthUsername, "basic auth user name")
	cmd.Flags().StringVar(&a.basicAuthPassword, "auth-pass", a.basicAuthPassword, "basic auth password")
	cmd.Flags().StringVar(&a.shareBasePath, "share-base-path", a.shareBasePath, "the http base path to serve share links")
	cmd.Flags().StringArrayVar(&a.shares, "share", a.shares, "serve a fixed path at <share-base-path>/<token>, format <token>=<path>, repeatable")
	cmd.Flags().StringArrayVar(&a.shareGens, "share-gen", a.shareGens, "serve a fixed path at <share-base-path>/<random token>, format <path>, repeatable")
	cmd.Flags().StringSliceVar(&a.allowOrigins, "allow-origins", a.allowOrigins, "allow origins")
	cmd.Flags().StringSliceVar(&a.allowMethods, "allow-methods", a.allowMethods, "allow methods")
	cmd.Flags().StringSliceVar(&a.allowHeaders, "allow-headers", a.allowHeaders, "allow headers")
	cmd.Flags().StringSliceVar(&a.exposeHeaders, "expose-headers", a.exposeHeaders, "expose headers")
	cmd.Flags().BoolVar(&a.allowCredentials, "allow-cres", a.allowCredentials, "allow credentials")

	return cmd
}

func runServer(a *args) {
	// Create gin engine
	ge := gin.New()
	ge.Use(gin.LoggerWithFormatter(logFormatter))
	ge.Use(gin.Recovery())
	ge.Use(gzip.Gzip(gzip.DefaultCompression))
	ge.Use(cors.New(cors.Config{
		AllowOrigins:     a.allowOrigins,
		AllowMethods:     a.allowMethods,
		AllowHeaders:     a.allowHeaders,
		ExposeHeaders:    a.exposeHeaders,
		AllowCredentials: a.allowCredentials,
		MaxAge:           12 * time.Hour,
	}))

	g := ge.Group(a.staticBasePath)
	g.Static("/", a.staticAssetDir)

	if a.basicAuthAssertDir != "" {
		ag := ge.Group(a.basicAuthBasePath, gin.BasicAuth(gin.Accounts{a.basicAuthUsername: a.basicAuthPassword}))
		ag.Static("/", a.basicAuthAssertDir)
	}

	shares, err := resolveShares(a)
	if err != nil {
		log.Fatalln(err)
	}
	if len(shares) != 0 {
		sg := ge.Group(a.shareBasePath)
		for _, s := range shares {
			fn := sg.StaticFile
			if s.isDir {
				fn = sg.Static
			}
			fn("/"+s.token, s.path)
			log.Printf("share %s -> %s", path.Join(a.shareBasePath, s.token), s.path)
		}
	}

	if err := http.ListenAndServe(a.listenAddr, ge); err != nil {
		log.Fatalln(err)
	}
}

type share struct {
	token string
	path  string
	isDir bool
}

// resolveShares turns the --share/--share-gen flags into a token -> path table,
// generating a random token for every --share-gen entry.
func resolveShares(a *args) ([]share, error) {
	var out []share
	seen := map[string]bool{}
	add := func(token, p string) error {
		if token == "" || p == "" {
			return fmt.Errorf("invalid share %q=%q, both token and path are required", token, p)
		}
		// ':' and '*' are gin route parameter prefixes, a token starting with
		// either turns the share into a catch-all route or panics outright.
		if strings.ContainsAny(token, "/?#:*") || strings.ContainsAny(token, " \t") {
			return fmt.Errorf("invalid share token %q, '/', '?', '#', ':', '*' and spaces are not allowed", token)
		}
		if token == "." || token == ".." {
			return fmt.Errorf("invalid share token %q", token)
		}
		if seen[token] {
			return fmt.Errorf("duplicated share token %q", token)
		}
		fi, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("share token %q: %v", token, err)
		}
		seen[token] = true
		out = append(out, share{token: token, path: p, isDir: fi.IsDir()})
		return nil
	}
	for _, it := range a.shares {
		token, p, ok := strings.Cut(it, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --share %q, expecting <token>=<path>", it)
		}
		if err := add(token, p); err != nil {
			return nil, err
		}
	}
	for _, p := range a.shareGens {
		token, err := randToken()
		if err != nil {
			return nil, err
		}
		if err := add(token, p); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func randToken() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func main() {
	if err := cmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
}

var logFormatter = func(param gin.LogFormatterParams) string {
	var statusColor, methodColor, resetColor string
	if param.IsOutputColor() {
		statusColor = param.StatusCodeColor()
		methodColor = param.MethodColor()
		resetColor = param.ResetColor()
	}

	if param.Latency > time.Minute {
		// Truncate in a golang < 1.8 safe way
		param.Latency = param.Latency - param.Latency%time.Second
	}
	return fmt.Sprintf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %s %s\n%s",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		statusColor, param.StatusCode, resetColor,
		param.Latency,
		param.ClientIP,
		methodColor, param.Method, resetColor,
		param.Request.UserAgent(),
		param.Path,
		param.ErrorMessage,
	)
}
