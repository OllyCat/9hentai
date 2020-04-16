package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	htmlquery "github.com/antchfx/xquery/html"
)

func main() {
	// в качестве параметра принимаем либо url, либо ключ
	if len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Printf("Use: %s <url>\n\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	// url должен иметь вид:
	// https://9hentai.com/g/60037/

	var phurl string
	// если запущено без параметров - читаем url из ввода
	if len(os.Args) < 2 {
		fmt.Print("Введите URL: ")
		fmt.Scan(&phurl)
	} else {
		// если задан - из аргументов строки
		phurl = os.Args[1]
	}

	// парсим url
	u, err := url.Parse(phurl)
	if err != nil {
		log.Fatal("URL incorrect: ", err)
	}

	// получаем ключ
	p := strings.Split(u.Path, "/")
	if len(p) != 4 {
		log.Fatal("URL incorrect: ", err)
	}

	viewkey := p[len(p)-2]

	// запрашиваем страницу
	resp, err := http.Get(phurl)
	if err != nil {
		log.Fatal("Get URL error: ", err)
	}

	// получаем тело страницы
	// парсим html
	defer resp.Body.Close()
	doc, err := htmlquery.Parse(resp.Body)
	if err != nil {
		log.Fatal("Parse HTML Error: ", err)
	}

	// получаем название манги //*[@id="info"]/h1
	title := viewkey
	titleNode := htmlquery.Find(doc, "//*[@id=\"info\"]/h1")
	if len(titleNode) > 0 {
		title = titleNode[0].FirstChild.Data
	} else {
		fmt.Println("Title not found, use viewkey.")
	}
	fmt.Println("Title: ", title)

	// выгребаем количество страниц
	// xpath //*[@id="info"]/div[1]
	pCountNode := htmlquery.Find(doc, "//*[@id=\"info\"]/div[1]")
	if len(pCountNode) < 1 {
		log.Fatal("Script not found.")
	}

	pCountText := pCountNode[0].FirstChild.Data
	p = strings.Split(pCountText, " ")
	pCount, err := strconv.Atoi(p[0])

	if err != nil {
		log.Fatal("Can't convert to int.")
	}

	// закачка

	// создаём директорий
	err = os.Mkdir(title, 0750)
	if err != nil && !os.IsExist(err) {
		log.Fatal("Can't make dir: ", err)
	}

	// переходим в него
	err = os.Chdir(title)
	if err != nil {
		log.Fatal("Can't change dir.")
	}

	// запускаем рутины на каждый файл закачки и ждём, пока они закончатся
	picsUrl := "https://cdn.9hentai.com/images/" + viewkey

	var wg sync.WaitGroup

	for i := 1; i <= pCount; i++ {
		fName := fmt.Sprint(i) + ".jpg"
		pUrl := picsUrl + "/" + fName

		wg.Add(1)

		go func(u string, fName string) {
			defer wg.Done()
			//fmt.Println("Downloading: ", u)
			resp, err := http.Get(u)
			if err != nil {
				return
			}
			//fmt.Println("Status: ", resp.Status)

			defer resp.Body.Close()
			f, err := os.OpenFile(fName, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Println("Can't create file: ", fName)
				return
			}

			if _, err = io.Copy(f, resp.Body); err != nil {
				fmt.Printf("Can't download file %v, error: %v\n", fName, err)
			}
			log.Printf("Done downloading file %v\n", fName)
		}(pUrl, fName)
	}
	fmt.Printf("Waiting...")
	wg.Wait()
}
