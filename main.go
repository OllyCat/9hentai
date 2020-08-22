package main

import (
	"bufio"
	"flag"
	"log"
	"os"
)

func main() {
	var dl DownStruct

	// ключ для количества потоков
	flag.IntVar(&dl.streams, "s", 10, "number of streams")

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

			// закачка
			if err := dl.Download(phurl); err != nil {
				log.Printf("%v\n", err)
				continue
			}
		}
		os.Exit(0)
	}
	// если заданы url-ы в ком строке, то итерируемся по ним
	for _, phurl = range flag.Args() {

		// закачка
		if err := dl.Download(phurl); err != nil {
			log.Printf("%v\n", err)
			continue
		}
	}
	os.Exit(0)
}
