package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	htmlquery "github.com/antchfx/xquery/html"
	pb "github.com/schollz/progressbar"
)

type DownStruct struct {
	bookId  string
	pCount  int
	title   string
	mUrl    string
	streams int
	bar     *pb.ProgressBar
	wg      sync.WaitGroup
}

func (d *DownStruct) getParam(u string) error {
	// url должен иметь вид:
	// https://9hentai.com/g/600/

	// находим bookid или вернём ошибку
	if err := d.GetBookId(u); err != nil {
		return err
	}

	// находим название, или ошибка
	if err := d.GetTitle(); err != nil {
		return err
	}

	// вернём nil если всё хорошо
	return nil
}

func (d *DownStruct) GetBookId(u string) error {
	// получаем ключ
	r := regexp.MustCompile("http.*9hentai.com/g/([0-9]+)")
	p := r.FindStringSubmatch(u)

	if len(p) != 2 {
		return fmt.Errorf("URL has not key in path: '%v'", d.mUrl)
	}

	d.mUrl = p[0]
	d.bookId = p[1]
	return nil
}

func (d *DownStruct) GetTitle() error {
	// запрашиваем страницу
	resp, err := http.Get(d.mUrl)
	if err != nil {
		return err
	}

	// получаем тело страницы
	// парсим html
	defer resp.Body.Close()
	doc, err := htmlquery.Parse(resp.Body)
	if err != nil {
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
		return errors.New("Pages not found.")
	}

	pCountText := pCountNode[0].FirstChild.Data
	p := strings.Split(pCountText, " ")
	d.pCount, err = strconv.Atoi(p[0])

	if err != nil {
		return fmt.Errorf("Can't convert to int: %w", err)
	}
	return nil
}

func (d *DownStruct) Download(phurl string) error {

	// получаем параметры
	if err := d.getParam(phurl); err != nil {
		return err
	}

	// если файл уже существует - выходим
	_, err := os.Stat(d.title + ".cbz")
	if err == nil {
		return errors.New("Comix already exist")
	}

	rand.Seed(time.Now().UnixNano())

	// создаём директорий
	err = os.Mkdir(d.title, 0750)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Can't make dir: %w", err)
	}

	// переходим в него
	err = os.Chdir(d.title)
	if err != nil {
		return fmt.Errorf("Can't change dir: %w", err)
	}

	// запускаем рутины на каждый файл закачки и ждём, пока они закончатся
	picsUrl := "https://cdn.9hentai.com/images/" + d.bookId

	d.bar = pb.New(d.pCount)
	d.bar.Describe("Downloading:")
	err = d.bar.RenderBlank()

	if err != nil {
		return fmt.Errorf("Error render bar: %w", err)
	}

	// канал для ограничения количества одновременных закачек
	c := make(chan int, d.streams)

	for i := 1; i <= d.pCount; i++ {

		d.wg.Add(1)

		// пишем в канал, если он полон - ожидаем пока не освободится
		c <- i
		// go routin-а на скачивание
		go func(picsUrl string, i int) {
			// освобождаем канал перед выходом
			defer func() {
				<-c
			}()

			// формируем ссылку на картинку
			fName := fmt.Sprint(i) + ".jpg"
			u := picsUrl + "/" + fName

			// обновляем бар перед выходом
			defer d.bar.Add(1)
			// дефер для завершения wg
			defer d.wg.Done()

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

				// если ответ сервера больше 404 - то нечего ловить, выходим
				if resp.StatusCode == 404 {
					return
				}

				// если же нет - подождём немного и снова запросим
				// это нужно, так как часто получаем html в качестве ответа из-за сильной загрузки сервера
				// если за RETR попыток не удалось - выходим, что бы не зависнуть совсем
				if retr <= 0 {
					log.Printf("Can't download %s file after %d retry.", fName, retr)
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
					// обновляем бар перед выходом
					d.bar.Add(1)
					return
				}
				// если не совпадает - удалим и пойдём перекачивать
				err = os.Remove(fName)
				if err != nil {
					log.Printf("The file size does not match. Error delete old file '%s'\n", fName)
					return
				}
			}

			f, err := os.Create(fName)
			if err != nil {
				log.Println("Can't create file: ", fName)
				return
			}

			if _, err = io.Copy(f, resp.Body); err != nil {
				log.Printf("Can't download file %v, error: %v\n", fName, err)
			}
		}(picsUrl, i)
	}
	d.wg.Wait()
	fmt.Println()
	os.Chdir("..")
	return d.compress()
}

func (d *DownStruct) compress() error {
	d.bar.Describe("Compression:")
	d.bar.Set(1)
	err := d.bar.RenderBlank()

	if err != nil {
		return fmt.Errorf("Error render bar: %w", err)
	}

	f, err := os.Create(d.title + ".cbz")
	if err != nil {
		return fmt.Errorf("Could not create archive: %w", err)
	}
	defer f.Close()

	z := zip.NewWriter(f)
	defer z.Close()

	err = filepath.Walk(d.title, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		d.bar.Add(1)

		if info.IsDir() {
			return nil
		}

		rf, e := os.Open(path)
		if e != nil {
			return fmt.Errorf("Error open file: %w", e)
		}
		defer rf.Close()

		zf, e := z.Create(path)
		if e != nil {
			return fmt.Errorf("Error archive file: %w", e)
		}

		_, e = io.Copy(zf, rf)
		if e != nil {
			return fmt.Errorf("Error copy file: %w", e)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("Error create cbz file: %w", err)
	}

	err = os.RemoveAll(d.title)
	if err != nil {
		return fmt.Errorf("Error remove original dir: %w", err)
	}

	fmt.Println()
	return nil
}
