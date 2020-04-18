package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var DEBUG bool

func main() {
	var dl dnldr

	// включаем дебагинг сообщений
	//dl.setDebug()

	// в качестве параметра принимаем либо url, либо ключ
	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Printf("Use: %s <url>\n\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	var phurl string
	// если запущено без параметров - читаем url из ввода
	if len(os.Args) < 2 {
		fmt.Print("Enter URL: ")
		fmt.Scan(&phurl)
	} else {
		// если задан - из аргументов строки
		phurl = os.Args[1]
	}

	dl.getParam(phurl)

	// закачка
	dl.download()
}
