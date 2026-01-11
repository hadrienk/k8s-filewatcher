package filewatcher_test

import (
	"context"
	"fmt"
	"log"
	"time"

	filewatcher "github.com/hadrienk/k8s-filewatcher"
)

func ExampleWatcher() {
	watcher, err := filewatcher.New("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		filewatcher.WithInterval(5*time.Second),
		filewatcher.WithOnChange(func(content []byte) {
			log.Printf("CA certificate reloaded, %d bytes", len(content))
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	go watcher.Start(ctx)

	caCert := watcher.Get()
	fmt.Printf("Current CA cert size: %d bytes\n", len(caCert))
}
