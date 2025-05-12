package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
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
	cmd.Flags().StringVar(&a.basicAuthAssertDir, "auth-asset-dir", a.staticAssetDir, "the basic auth enabled static site asset dir")
	cmd.Flags().StringVar(&a.basicAuthUsername, "auth-user", a.basicAuthUsername, "basic auth user name")
	cmd.Flags().StringVar(&a.basicAuthPassword, "auth-pass", a.basicAuthPassword, "basic auth password")
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

	err := http.ListenAndServe(a.listenAddr, ge)
	if err != nil {
		log.Fatalln(err)
	}
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
