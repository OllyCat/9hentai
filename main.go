package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

var DEBUG bool

func main() {
	var dl dnldr

	// дебагинг сообщений
	DEBUG = false

	// в качестве параметра принимаем либо url, либо ключ
	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Printf("Use: %s <url>\n\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	var phurl string
	// если запущено без параметров - читаем url из ввода
	if len(os.Args) < 2 {
		scanner := bufio.NewScanner(os.Stdin)
		for {
			ok := scanner.Scan()
			if ok {
				phurl = scanner.Text()
				if len(phurl) == 0 {
					continue
				}
				if err := dl.getParam(phurl); err != nil {
					log.Printf("Error: %v\n", err)
					continue
				}
				// закачка
				if err := dl.download(); err != nil {
					log.Printf("Error: %v\n", err)
				}
			} else {
				break
			}
		}
	} else {
		// если задан - из аргументов строки
		phurl = os.Args[1]

		if err := dl.getParam(phurl); err != nil {
			log.Printf("Error: %v\n", err)
		}
		// закачка
		if err := dl.download(); err != nil {
			log.Printf("Error: %v\n", err)
		}
	}
}
