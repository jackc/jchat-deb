package main

import (
	"errors"
	"fmt"
	"github.com/jackc/cli"
	"github.com/vaughan0/go-ini"
	"golang.org/x/net/websocket"
	log "gopkg.in/inconshreveable/log15.v2"
	"net/http"
	"net/http/httputil"
	"net/smtp"
	"net/url"
	"os"
	"path/filepath"
)

const version = "0.0.1"

type httpConfig struct {
	listenAddress string
	listenPort    string
	staticURL     string
}

func main() {
	app := cli.NewApp()
	app.Name = "jchat"
	app.Usage = "JChat"
	app.Version = version
	app.Author = "Jack Christensen"
	app.Email = "jack@jackchristensen.com"

	app.Commands = []cli.Command{
		{
			Name:        "server",
			ShortName:   "s",
			Usage:       "run the server",
			Synopsis:    "[command options]",
			Description: "run the jchat server",
			Flags: []cli.Flag{
				cli.StringFlag{"address, a", "127.0.0.1", "address to listen on"},
				cli.StringFlag{"port, p", "8080", "port to listen on"},
				cli.StringFlag{"config, c", "jchat.conf", "path to config file"},
				cli.StringFlag{"static-url", "", "reverse proxy static asset requests to URL"},
			},
			Action: Serve,
		},
	}

	app.Run(os.Args)

}

func loadConfig(path string) (ini.File, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("Invalid config path: %v", err)
	}

	file, err := ini.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to load config file: %v", err)
	}

	return file, nil
}

func newLogger(conf ini.File) (log.Logger, error) {
	level, _ := conf.Get("log", "level")
	if level == "" {
		level = "warn"
	}

	logger := log.New()
	setFilterHandler(level, logger, log.StdoutHandler)

	return logger, nil
}

func setFilterHandler(level string, logger log.Logger, handler log.Handler) error {
	if level == "none" {
		logger.SetHandler(log.DiscardHandler())
		return nil
	}

	lvl, err := log.LvlFromString(level)
	if err != nil {
		return fmt.Errorf("Bad log level: %v", err)
	}
	logger.SetHandler(log.LvlFilterHandler(lvl, handler))

	return nil
}

func newRepo(conf ini.File, logger log.Logger) (UserRepository, error) {
	pool, err := newConnPool(conf)
	if err != nil {
		return nil, fmt.Errorf("Unable to create pgx connection pool: %v", err)
	}

	repo := NewPgxUserRepository(pool)
	return repo, nil
}

func loadHTTPConfig(c *cli.Context, conf ini.File) (httpConfig, error) {
	config := httpConfig{}
	config.listenAddress = c.String("address")
	config.listenPort = c.String("port")
	config.staticURL = c.String("static-url")

	var ok bool
	if !c.IsSet("address") {
		if config.listenAddress, ok = conf.Get("server", "address"); !ok {
			return config, errors.New("Missing server address")
		}
	}

	if !c.IsSet("port") {
		if config.listenPort, ok = conf.Get("server", "port"); !ok {
			return config, errors.New("Missing server port")
		}
	}

	return config, nil
}

func newMailer(conf ini.File, logger log.Logger) (Mailer, error) {
	mailConf := conf.Section("mail")
	if len(mailConf) == 0 {
		return nil, nil
	}

	smtpAddr, ok := mailConf["smtp_server"]
	if !ok {
		return nil, errors.New("Missing mail -- smtp_server")
	}
	smtpPort, _ := mailConf["port"]
	if smtpPort == "" {
		smtpPort = "587"
	}

	fromAddr, ok := mailConf["from_address"]
	if !ok {
		return nil, errors.New("Missing mail -- from_address")
	}

	rootURL, ok := mailConf["root_url"]
	if !ok {
		return nil, errors.New("Missing mail -- root_url")
	}

	username, _ := mailConf["username"]
	password, _ := mailConf["password"]

	auth := smtp.PlainAuth("", username, password, smtpAddr)

	logger = logger.New("module", "mail")

	mailer := &SMTPMailer{
		ServerAddr: smtpAddr + ":" + smtpPort,
		Auth:       auth,
		From:       fromAddr,
		rootURL:    rootURL,
		logger:     logger,
	}

	return mailer, nil
}

func Serve(c *cli.Context) {
	conf, err := loadConfig(c.String("config"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	httpConfig, err := loadHTTPConfig(c, conf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	logger, err := newLogger(conf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	pool, err := newConnPool(conf)
	if err != nil {
		fmt.Println("Unable to create pgx connection pool: %v", err)
		os.Exit(1)
	}

	userRepo := NewPgxUserRepository(pool)
	sessionRepo := NewPgxSessionRepository(pool)
	chatRepo := NewPgxChatRepository(pool)

	mailer, err := newMailer(conf, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if httpConfig.staticURL != "" {
		staticURL, err := url.Parse(httpConfig.staticURL)
		if err != nil {
			logger.Crit(fmt.Sprintf("Bad static-url: %v", err))
			os.Exit(1)
		}
		http.Handle("/", httputil.NewSingleHostReverseProxy(staticURL))
	}

	http.Handle("/ws", websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

		conn := &ClientConn{
			ws:          ws,
			userRepo:    userRepo,
			sessionRepo: sessionRepo,
			chatRepo:    chatRepo,
			logger:      logger,
			mailer:      mailer,
		}

		conn.Dispatch()
	}))

	listenAt := fmt.Sprintf("%s:%s", httpConfig.listenAddress, httpConfig.listenPort)
	fmt.Printf("Starting to listen on: %s\n", listenAt)

	if err := http.ListenAndServe(listenAt, nil); err != nil {
		os.Stderr.WriteString("Could not start web server!\n")
		os.Exit(1)
	}
}
