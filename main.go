package main

import (
	"log"
	"os"
	"sync"

	"github.com/payfazz/go-errors"
	"github.com/payfazz/go-errors/errhandler"

	"github.com/payfazz/socktunrouter/internal/config"
	"github.com/payfazz/socktunrouter/internal/done"
	"github.com/payfazz/socktunrouter/internal/tun"
)

func main() {
	var err error

	infLog := log.New(os.Stdout, "INF: ", log.LstdFlags)
	errLog := log.New(os.Stdout, "ERR: ", log.LstdFlags)

	defer errhandler.With(func(err error) {
		errLog.Println(errors.Format(err))
		os.Exit(1)
	})

	if len(os.Args) != 2 {
		errhandler.Check(errors.Errorf("Usage: %s <config.yml>", os.Args[0]))
	}

	config, err := config.Parse(os.Args[1])
	errhandler.Check(errors.Wrap(err))

	if config.TunName == "" {
		errhandler.Check(errors.Errorf("tun name cannot be empty"))
	}

	tunDev, err := tun.Open(config.TunName)
	errhandler.Check(errors.Wrap(err))
	defer tunDev.Close()

	if tunDev.Name() != config.TunName {
		infLog.Printf("Opened tun device: %s\n", tunDev.Name())
	}

	done := done.New()
	interruptHandler(func(sig os.Signal) {
		infLog.Printf("Got signal %s ...\n", sig.String())
		done.Done()
	})

	wg := &sync.WaitGroup{}

	inputErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := inputMain(infLog, errLog, done, tunDev, config); err != nil {
			select {
			case inputErr <- err:
			case <-done.WaitCh():
			}
		}
	}()

	outputErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := outputMain(infLog, errLog, done, tunDev, config); err != nil {
			select {
			case outputErr <- err:
			case <-done.WaitCh():
			}
		}
	}()

	err = nil
	select {
	case err = <-inputErr:
	case err = <-outputErr:
	case <-done.WaitCh():
	}
	done.Done()

	wg.Wait()
	errhandler.Check(errors.Wrap(err))
}
