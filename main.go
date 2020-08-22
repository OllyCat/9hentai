package main

import (
	"bufio"
	"flag"
	"log"
	"os"
)

var DEBUG bool

func main() {
	var dl DownStruct

	// дебагинг сообщений
	flag.BoolVar(&DEBUG, "d", false, "for debug messages")

	// в качестве параметра принимаем либо url, либо ключ
	flag.Parse()

	var phurl string
	// если запущено без параметров - читаем url из ввода
	if len(flag.Args()) < 1 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
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

			// компрессия
			if err := dl.Compress(); err != nil {
				log.Printf("Error: %v\n", err)
			}
		}
		os.Exit(0)
	}
	// если заданы url-ы в ком строке, то итерируемся по ним
	for _, phurl = range flag.Args() {

		if err := dl.getParam(phurl); err != nil {
			log.Printf("Error: %v\n", err)
			continue
		}
		// закачка
		if err := dl.download(); err != nil {
			log.Printf("Error: %v\n", err)
		}
		// компрессия
		if err := dl.Compress(); err != nil {
			log.Printf("Error: %v\n", err)
		}
	}
}
