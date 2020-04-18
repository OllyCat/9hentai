package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	htmlquery "github.com/antchfx/xquery/html"
	pb "github.com/schollz/progressbar"
)

type dnldr struct {
	DEBUG  bool
	bookId string
	pCount int
	title  string
	mUrl   string
}

func (d *dnldr) setDebug() {
	d.DEBUG = true
}

func (d *dnldr) getParam(u string) {
	// url должен иметь вид:
	// https://9hentai.com/g/600/

	d.mUrl = u
	d.getBookId()
	d.getTitle()
}

func (d *dnldr) getBookId() {
	// парсим url
	u, err := url.Parse(d.mUrl)
	if err != nil {
		log.Fatal("URL incorrect: ", err)
	}

	// получаем ключ
	p := strings.Split(u.Path, "/")
	if len(p) != 4 {
		log.Fatal("URL incorrect: ", err)
	}

	d.bookId = p[len(p)-2]
}

func (d *dnldr) getTitle() {
	// запрашиваем страницу
	resp, err := http.Get(d.mUrl)
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
	d.title = d.bookId
	titleNode := htmlquery.Find(doc, "//*[@id=\"info\"]/h1")
	if len(titleNode) > 0 {
		d.title = titleNode[0].FirstChild.Data
	} else {
		log.Println("Title not found, use number.")
	}
	log.Println("Title: ", d.title)

	// заменим, если есть, символы разделения путей OS
	d.title = strings.ReplaceAll(d.title, string(os.PathSeparator), ".")

	// выгребаем количество страниц
	// xpath //*[@id="info"]/div[1]
	pCountNode := htmlquery.Find(doc, "//*[@id=\"info\"]/div[1]")
	if len(pCountNode) < 1 {
		log.Fatal("Pages not found.")
	}

	pCountText := pCountNode[0].FirstChild.Data
	p := strings.Split(pCountText, " ")
	d.pCount, err = strconv.Atoi(p[0])

	if err != nil {
		log.Fatal("Can't convert to int.")
	}
}

func (d *dnldr) download() {

	// создаём директорий
	err := os.Mkdir(d.title, 0750)
	if err != nil && !os.IsExist(err) {
		log.Fatal("Can't make dir: ", err)
	}

	// переходим в него
	err = os.Chdir(d.title)
	if err != nil {
		log.Fatal("Can't change dir.")
	}

	// запускаем рутины на каждый файл закачки и ждём, пока они закончатся
	picsUrl := "https://cdn.9hentai.com/images/" + d.bookId

	var wg sync.WaitGroup

	bar := pb.New(d.pCount)

	for i := 1; i <= d.pCount; i++ {
		// формируем ссылку на картинку
		fName := fmt.Sprint(i) + ".jpg"
		pUrl := picsUrl + "/" + fName

		wg.Add(1)

		// go routin-а на скачивание
		go func(u string, fName string) {
			// дефер для завершения wg
			defer wg.Done()

			var resp *http.Response
			var err error

			// цикл запроса к серверу
		LOOP:
			for retr := 10; retr > 0; retr-- {
				resp, err = http.Get(u)
				// выходим из рутины если ошибка
				if err != nil {
					return
				}

				// если контекст - картинка, то прерываемся, что бы сохранить в файл
				if strings.HasPrefix(resp.Header["Content-Type"][0], "image") {
					break LOOP
				}
				// закроем ответ от сервера
				resp.Body.Close()
				// если же нет - подождём немного и снова запросим
				// это нужно, так как часто получаем html в качестве ответа из-за сильной загрузки сервера
				// если за RETR попыток не удалось - выходим, что бы не зависнуть совсем
				d.Debug(fmt.Sprintf("Retry %v of file '%v'\n", 10-retr, fName))
				if retr <= 0 {
					log.Printf("Can't download %s file after %d retry.\n", fName, retr)
					return
				}
				//time.Sleep(100 * time.Millisecond)
			}

			defer resp.Body.Close()

			// проверим размер ответа
			cLen := resp.ContentLength
			if cLen == 0 {
				log.Printf("Content length of file '%s' is nil.\n", fName)
				return
			}

			// смотрим есть ли такой файл уже на диске
			if stat, err := os.Stat(fName); err == nil {
				// если есть - смотрим размер
				fSize := stat.Size()
				// совпадает с Content-Length - смело выходим
				if fSize == cLen {
					d.Debug(fmt.Sprintf("File '%s' already exist.\n", fName))
					// обновляем бар перед выходом
					bar.Add(1)
					return
				}
				// если не совпадает - удалим и пойдём перекачивать
				err = os.Remove(fName)
				if err != nil {
					log.Printf("The file size does not match. Error delete old file '%s'\n", fName)
					return
				}
			}

			f, err := os.OpenFile(fName, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Println("Can't create file: ", fName)
				return
			}

			if _, err = io.Copy(f, resp.Body); err != nil {
				log.Printf("Can't download file %v, error: %v\n", fName, err)
			}
			d.Debug(fmt.Sprintf("Done downloading file %v\n", fName))
			// обновляем бар перед выходом
			bar.Add(1)
		}(pUrl, fName)
	}
	wg.Wait()

}

func (d *dnldr) Debug(s string) {
	if d.DEBUG {
		log.Println(s)
	}
}
