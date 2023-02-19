package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	defer notifyStop()

	var (
		dir string
	)
	flag.StringVar(&dir, "dir", "/var/log/syslogd",
		"directory where syslog files are written")
	flag.Parse()

	log.SetPrefix("# ")
	if err := main1(dir); err != nil {
		panic(err)
	}
}

func main1(dir string) error {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGHUP)
	defer cancel()

	lg := &lumberjack.Logger{
		Filename:   filepath.Join(dir, "syslog"),
		MaxSize:    500,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}
	defer lg.Close()

	w := bufio.NewWriter(lg)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: 514,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	notifyReady()

	buf := make([]byte, 65_535)
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}
			log.Printf("ReadFrom: %v", err)
			continue
		}
		line := buf[:n]

		if _, err := w.Write(line); err != nil {
			log.Printf("unable to write: %v", err)
			continue
		}
		if len(line) > 0 && line[len(line)-1] != '\n' {
			if err := w.WriteByte('\n'); err != nil {
				log.Printf("unable to write newline: %v", err)
				continue
			}
		}
	}
	w.Flush()
	return lg.Close()
}

func notifyReady() error {
	s := os.Getenv("NOTIFY_SOCKET")
	if s == "" {
		return nil
	}
	return sdNotify(s, "READY=1")
}

func notifyStop() error {
	s := os.Getenv("NOTIFY_SOCKET")
	if s == "" {
		return nil
	}
	return sdNotify(s, "STOPPING=1")
}

func sdNotify(path, payload string) error {
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{
		Name: path,
		Net:  "unixgram",
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = io.Copy(conn, strings.NewReader(payload))
	return err
}
