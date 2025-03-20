package main

import (
	"encoding/json"
	"go-playwright/internal/common"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/playwright-community/playwright-go"
	"github.com/spf13/viper"
)

func main() {
	common.LoadConfig("env")

	js, err := connectNATS(viper.GetString("nats-server.endpoint"))
	if err != nil {
		log.Fatalf("[main] connectNATS, %v", err)
	}

	if err := ensureRuntimeDir(); err != nil {
		log.Fatalf("[main] ensureRuntimeDir, %v", err)
	}

	go func(js nats.JetStreamContext) {
		js.QueueSubscribe(viper.GetString("nats-server.subject"), "worker", process)
	}(js)

	runtime.Goexit()
}

func connectNATS(host string) (nats.JetStreamContext, error) {
	nc, err := nats.Connect(
		"nats://"+host+":"+viper.GetString("nats-server.port"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(20),
		nats.ReconnectWait(time.Second*5),
		nats.DisconnectErrHandler(func(c *nats.Conn, err error) {
			log.Printf("[nats][%s]Got disconnected! Reason: %q\n", host, err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("[nats][%s]Got reconnected to %v!\n", host, nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			log.Printf("[nats][%s]Connection closed. Reason: %q\n", host, nc.LastError())
		}),
	)
	if err != nil {
		return nil, err
	}

	return nc.JetStream()
}

func ensureRuntimeDir() error {
	runtimeDir := "runtime"
	info, err := os.Stat(runtimeDir)
	if os.IsNotExist(err) {
		return os.Mkdir(runtimeDir, 0755)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return os.ErrExist
	}
	return nil
}

type Task struct {
	Uri string `json:"uri"`
}

func process(msg *nats.Msg) {
	// recover
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[process][recover] %v, task: %s", r, string(msg.Data))
		}

		msg.Ack()
	}()

	var task Task
	err := json.Unmarshal(msg.Data, &task)
	if err != nil {
		log.Fatalf("[process] json.Unmarshal, error: %v", err)
	}

	result := run(task.Uri)
	log.Printf("crawler %s to %s", task.Uri, result)

	msg.Ack()
}

func run(uri string) string {
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("[main] playwright.Run, error: %v", err)
	}
	defer func() {
		if err = pw.Stop(); err != nil {
			log.Fatalf("[main] pw.Stop, error: %v", err)
		}
	}()

	browser, err := pw.Firefox.Launch(playwright.BrowserTypeLaunchOptions{
		ExecutablePath: playwright.String(viper.GetString("playwright.executable-path")),
	})
	if err != nil {
		log.Fatalf("[main] pw.Firefox.Launch, error: %v", err)
	}
	defer func() {
		if err = browser.Close(); err != nil {
			log.Fatalf("[main] browser.Close, error: %v", err)
		}
	}()

	page, err := browser.NewPage()
	if err != nil {
		log.Fatalf("[main] browser.NewPage, error: %v", err)
	}

	page.On("framenavigated", func(frame playwright.Frame) {
		log.Printf("page reloaded: %s\n", frame.URL())
	})

	_, err = page.Goto(uri, playwright.PageGotoOptions{
		WaitUntil: (*playwright.WaitUntilState)(playwright.String("networkidle")),
	})
	// _, err = page.Goto("https://playwright.dev/")
	if err != nil {
		log.Fatalf("[main] page.Goto, error: %v", err)
	}

	html, err := page.Content()
	if err != nil {
		log.Fatalf("[main] page.Content, error: %v", err)
	}

	filename := uuid.New().String() + ".html"
	err = os.WriteFile("runtime/"+filename, []byte(html), 0644)
	if err != nil {
		log.Fatalf("[main] os.WriteFile, error: %v", err)
	}

	return filename
}
