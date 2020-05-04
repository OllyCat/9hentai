package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	htmlquery "github.com/antchfx/xquery/html"
	pb "github.com/schollz/progressbar"
)

type DownStruct struct {
	bookId string
	pCount int
	title  string
	mUrl   string
}

func (d *DownStruct) getParam(u string) error {
	// url должен иметь вид:
	// https://9hentai.com/g/600/

	// находим bookid или вернём ошибку
	if err := d.getBookId(u); err != nil {
		return err
	}

	// находим название, или ошибка
	if err := d.getTitle(); err != nil {
		return err
	}

	// вернём nil если всё хорошо
	return nil
}

func (d *DownStruct) getBookId(u string) error {
	// получаем ключ
	r := regexp.MustCompile("http.*9hentai.com/g/([0-9]+)")
	p := r.FindStringSubmatch(u)

	if len(p) != 2 {
		return errors.New(fmt.Sprintf("URL has not key in path: '%v'", d.mUrl))
	}

	d.mUrl = p[0]
	d.bookId = p[1]
	return nil
}

func (d *DownStruct) getTitle() error {
	// запрашиваем страницу
	resp, err := http.Get(d.mUrl)
	if err != nil {
		Debug(fmt.Sprintln("Get URL error: ", err))
		return err
	}

	// получаем тело страницы
	// парсим html
	defer resp.Body.Close()
	doc, err := htmlquery.Parse(resp.Body)
	if err != nil {
		Debug(fmt.Sprintln("Parse HTML error: ", err))
		return err
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
		Debug("Pages not found.")
		return errors.New("Pages not found.")
	}

	pCountText := pCountNode[0].FirstChild.Data
	p := strings.Split(pCountText, " ")
	d.pCount, err = strconv.Atoi(p[0])

	if err != nil {
		Debug("Can't convert to int.")
		return errors.New("Can't convert to int.")
	}
	return nil
}

func (d *DownStruct) download() error {

	rand.Seed(time.Now().UnixNano())

	// создаём директорий
	err := os.Mkdir(d.title, 0750)
	if err != nil && !os.IsExist(err) {
		Debug(fmt.Sprintln("Can't make dir: ", err))
		return err
	}

	// переходим в него
	err = os.Chdir(d.title)
	if err != nil {
		Debug("Can't change dir.")
		return errors.New("Can't change dir.")
	}
	defer os.Chdir("..")

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
			for retr := 100; retr > 0; retr-- {
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
				Debug(fmt.Sprintf("Retry %v of file '%v'\n", 10-retr, fName))
				if retr <= 0 {
					log.Printf("Can't download %s file after %d retry.\n", fName, retr)
					return
				}
				time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
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
					Debug(fmt.Sprintf("File '%s' already exist.\n", fName))
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
			Debug(fmt.Sprintf("Done downloading file %v\n", fName))
			// обновляем бар перед выходом
			bar.Add(1)
		}(pUrl, fName)
	}
	wg.Wait()
	fmt.Println()
	return nil
}

func Debug(s string) {
	if DEBUG {
		log.Println(s)
	}
}
