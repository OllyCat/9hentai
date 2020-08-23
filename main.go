package main

import (
	"bufio"
	"log"
	"os"

	"github.com/spf13/pflag"
)

func main() {
	var dl DownStruct

	// ключ для количества потоков
	pflag.IntVarP(&dl.streams, "strams", "s", 10, "number of streams")

	// help
	help := pflag.BoolP("help", "h", false, "help")

	pflag.Parse()

	if *help {
		pflag.Usage()
		os.Exit(0)
	}

	var phurl string
	// если запущено без параметров - читаем url из ввода
	if len(pflag.Args()) < 1 {
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
	for _, phurl = range pflag.Args() {

		// закачка
		if err := dl.Download(phurl); err != nil {
			log.Printf("%v\n", err)
			continue
		}
	}
	os.Exit(0)
}
