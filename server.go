package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/TeaMeow/KitSvc/module/sd"
	"github.com/TeaMeow/KitSvc/router"
	"github.com/TeaMeow/KitSvc/router/middleware"
	"github.com/TeaMeow/KitSvc/shared/eventutil"
	"github.com/TeaMeow/KitSvc/shared/mqutil"
	"github.com/TeaMeow/KitSvc/shared/wsutil"
	"github.com/codegangsta/cli"
	"github.com/gin-gonic/gin"
)

var serverFlags = []cli.Flag{
	// Common flags.
	cli.StringFlag{
		EnvVar: "KITSVC_NAME",
		Name:   "name",
		Usage:  "the name of the service, exposed for service discovery.",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_URL",
		Name:   "url",
		Usage:  "the url of the service.",
		Value:  "http://127.0.0.1:8080",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_ADDR",
		Name:   "addr",
		Usage:  "the address of the service (with the port).",
		Value:  "127.0.0.1:8080",
	},
	cli.IntFlag{
		EnvVar: "KITSVC_PORT",
		Name:   "port",
		Usage:  "the port of the service.",
		Value:  8080,
	},
	cli.StringFlag{
		EnvVar: "KITSVC_USAGE",
		Name:   "usage",
		Usage:  "the usage of the service, exposed for service discovery.",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_JWT_SECRET",
		Name:   "jwt-secret",
		Usage:  "the secert used to encode the json web token.",
	},
	cli.IntFlag{
		EnvVar: "KITSVC_MAX_PING_COUNT",
		Name:   "max-ping-count",
		Usage:  "",
		Value:  20,
	},

	// Database flags.
	cli.StringFlag{
		EnvVar: "KITSVC_DATABASE_NAME",
		Name:   "database-name",
		Usage:  "the name of the database.",
		Value:  "service",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_DATABASE_HOST",
		Name:   "database-host",
		Usage:  "the host of the database (with the port).",
		Value:  "127.0.0.1:3306",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_DATABASE_USER",
		Name:   "database-user",
		Usage:  "the user of the database.",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_DATABASE_PASSWORD",
		Name:   "database-password",
		Usage:  "the password of the database.",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_DATABASE_CHARSET",
		Name:   "database-charset",
		Usage:  "the charset of the database.",
		Value:  "utf8",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_DATABASE_LOC",
		Name:   "database-loc",
		Usage:  "the timezone of the database.",
		Value:  "Local",
	},
	cli.BoolFlag{
		EnvVar: "KITSVC_DATABASE_PARSE_TIME",
		Name:   "database-parse_time",
		Usage:  "parse the time.",
	},

	// NSQ flags.
	cli.StringFlag{
		EnvVar: "KITSVC_NSQ_PRODUCER",
		Name:   "nsq-producer",
		Usage:  "the address of the TCP NSQ producer (with the port).",
		Value:  "127.0.0.1:4150",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_NSQ_PRODUCER_HTTP",
		Name:   "nsq-producer-http",
		Usage:  "the address of the HTTP NSQ producer (with the port).",
		Value:  "127.0.0.1:4151",
	},
	cli.StringSliceFlag{
		EnvVar: "KITSVC_NSQ_LOOKUPDS",
		Name:   "nsq-lookupds",
		Usage:  "the address of the NSQ lookupds (with the port).",
		Value: &cli.StringSlice{
			"127.0.0.1:4161",
		},
	},

	// Event store flags.
	cli.StringFlag{
		EnvVar: "KITSVC_ES_SERVER_URL",
		Name:   "es-url",
		Usage:  "the url of the event store server.",
		Value:  "http://127.0.0.1:2113",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_ES_USERNAME",
		Name:   "es-username",
		Usage:  "the username of the event store.",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_ES_PASSWORD",
		Name:   "es-password",
		Usage:  "the password of the event store.",
	},

	// Prometheus flags.
	cli.StringFlag{
		EnvVar: "KITSVC_PROMETHEUS_NAMESPACE",
		Name:   "prometheus-namespace",
		Usage:  "the prometheus namespace.",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_PROMETHEUS_SUBSYSTEM",
		Name:   "prometheus-subsystem",
		Usage:  "the subsystem of the promethues.",
	},

	// Consul flags.
	cli.StringFlag{
		EnvVar: "KITSVC_CONSUL_CHECK_INTERVAL",
		Name:   "consul-check_interval",
		Usage:  "the interval of consul health check.",
		Value:  "10s",
	},
	cli.StringFlag{
		EnvVar: "KITSVC_CONSUL_CHECK_TIMEOUT",
		Name:   "consul-check_timeout",
		Usage:  "the timeout of consul health check.",
		Value:  "1s",
	},
	cli.StringSliceFlag{
		EnvVar: "KITSVC_CONSUL_TAGS",
		Name:   "consul-tags",
		Usage:  "the service tags for consul.",
		Value: &cli.StringSlice{
			"micro",
		},
	},
}

// server runs the server.
func server(c *cli.Context, started chan bool) error {
	// `deployed` will be closed when the router is deployed.
	// `replayed` will be closed after the events are all replayed.
	deployed := make(chan bool)
	replayed := make(chan bool)

	// Create the Gin engine.
	gin := gin.New()
	// Event handlers.
	event := eventutil.New(gin)
	// Websocket handlers.
	ws := wsutil.New(gin)
	// Message queue handlers.
	mq := mqutil.New(gin)

	// Routes.
	router.Load(
		// Cores.
		gin, event, ws, mq,
		// Middlwares.
		middleware.Config(c),
		middleware.Store(c),
		middleware.Logging(),
		middleware.Event(c, event, replayed, deployed),
		middleware.MQ(c, mq, deployed),
		middleware.Metrics(),
	)

	// Register to the service registry when the events were replayed.
	go func() {
		<-replayed

		sd.Register(c)
		// After the service is registered to the consul,
		// close the `started` channel to make it non-blocking.
		close(started)
	}()

	// Ping the server to make sure the router is working.
	go func() {
		if err := pingServer(c); err != nil {
			logrus.Fatalln("The router has no response, or it might took too long to start up.")
		}
		logrus.Infoln("The router has been deployed successfully.")
		// Close the `deployed` channel to make it non-blocking.
		close(deployed)
	}()

	// Start to listening the incoming requests.
	return http.ListenAndServe(c.String("addr"), gin)
}

// pingServer pings the http server to make sure the router is working.
func pingServer(c *cli.Context) error {
	for i := 0; i < c.Int("max-ping-count"); i++ {
		// Ping the server by sending a GET request to `/health`.
		resp, err := http.Get(c.String("url") + "/sd/health")
		if err == nil && resp.StatusCode == 200 {
			return nil
		}

		// Sleep for a second to continue the next ping.
		logrus.Infof("Waiting for the router, retry in 1 second.")
		time.Sleep(time.Second)
	}
	return errors.New("Cannot connect to the router.")
}
