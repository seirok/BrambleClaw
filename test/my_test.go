package test

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"
	"testing"
)

func TestLab(t *testing.T) {
	ctx, _ := context.WithCancel(context.Background())
	scanner := bufio.NewScanner(os.Stdin)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Print("> ")
			if scanner.Scan() {
				content := strings.TrimSpace(scanner.Text())
				if content != "" {
					log.Printf("message")

				}
			}
		}
	}
}
