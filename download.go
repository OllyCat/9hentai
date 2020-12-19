package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	htmlquery "github.com/antchfx/xquery/html"
	pb "github.com/schollz/progressbar"
	"github.com/valyala/fasthttp"
)

type DownStruct struct {
	bookId  string
	pCount  int
	title   string
	mUrl    string
	mDomain string
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
	r := regexp.MustCompile("http.*//(9hentai.+)/g/([0-9]+)")
	p := r.FindStringSubmatch(u)

	if len(p) != 3 {
		return fmt.Errorf("URL has not key in path: '%v'", d.mUrl)
	}

	d.mUrl = p[0]
	d.mDomain = p[1]
	d.bookId = p[2]
	return nil
}

func (d *DownStruct) GetTitle() error {
	var body []byte
	// запрашиваем страницу
	_, body, err := fasthttp.Get(nil, d.mUrl)
	if err != nil {
		return err
	}

	// получаем тело страницы
	// парсим html
	bReader := bytes.NewReader(body)
	doc, err := htmlquery.Parse(bReader)
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
	if len(p) < 1 {
		return errors.New("Pages count not found.")
	}
	d.pCount, err = strconv.Atoi(p[0])

	if err != nil {
		return fmt.Errorf("Can't convert to int: %w", err)
	}
	return nil
}

func (d *DownStruct) Download(phurl string) error {
	client := &fasthttp.Client{}

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
	picsUrl := "https://cdn." + d.mDomain + "/images/" + d.bookId

	// создаём бар
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

			var err error
			var resp *fasthttp.Response
			var req *fasthttp.Request

			// запрос к серверу
			// подготавливаем req и resp для fasthttp
			resp = fasthttp.AcquireResponse()
			req = fasthttp.AcquireRequest()
			req.Reset()
			resp.Reset()
			req.SetRequestURI(u)
			// запрос
			err = client.Do(req, resp)
			// выходим из рутины если ошибка
			if err != nil {
				log.Printf("Error: %v", err)
				return
			}

			// если ответ сервера больше 404 - то нечего ловить, выходим с сообщением
			if resp.StatusCode() == fasthttp.StatusNotFound {
				log.Printf("\nError: file %s does not exist.\n", fName)
				return
			}

			// если контекст - картинка, то прерываемся, что бы сохранить в файл
			content := resp.Header.ContentType()
			if !bytes.HasPrefix(content, []byte("image")) {
				log.Printf("\nDEBUG: Content type: %v in URL: %v\n", string(content), u)
				return
			}

			// проверим размер ответа
			cLen := resp.Header.ContentLength()
			if cLen <= 0 {
				log.Printf("Bad content length of file '%s'\n", fName)
				return
			}

			// смотрим есть ли такой файл уже на диске
			if stat, err := os.Stat(fName); err == nil {
				// если есть - смотрим размер
				fSize := stat.Size()
				// совпадает с Content-Length - смело выходим
				if fSize == int64(cLen) {
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

			// создаём файл
			f, err := os.Create(fName)
			if err != nil {
				log.Println("Can't create file: ", fName)
				return
			}
			defer f.Close()

			// качаем картинку
			code, body, err := fasthttp.Get(nil, u)
			if err != nil || code != 200 {
				log.Printf("Can't download file %v, error: %v, status code: %v\n", fName, err, code)
				return
			}
			// сохраняем картинку в файл
			f.Write(body)
		}(picsUrl, i)
	}
	// ожидаем завершения всех go рутин
	d.wg.Wait()
	fmt.Println()
	// вернёмся из подкаталога и сожмём всё
	os.Chdir("..")
	return d.compress()
}

func (d *DownStruct) compress() error {
	// бар для сжатия
	d.bar = pb.New(d.pCount)
	d.bar.Describe("Compression:")
	err := d.bar.RenderBlank()

	if err != nil {
		return fmt.Errorf("Error render bar: %w", err)
	}

	// создаём файл архива
	f, err := os.Create(d.title + ".cbz")
	if err != nil {
		return fmt.Errorf("Could not create archive: %w", err)
	}
	defer f.Close()

	// writer для zip архива
	z := zip.NewWriter(f)
	defer z.Close()

	// счётчик запакованных файлов
	var count int

	// проход по содержимому папки
	err = filepath.Walk(d.title, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// обновление бара на каждый файл
		d.bar.Add(1)

		// игнорируем, если директорий
		if info.IsDir() {
			return nil
		}

		// если файл - открываем его на чтение
		rf, e := os.Open(path)
		if e != nil {
			return fmt.Errorf("Error open file: %w", e)
		}
		// закрываем по окончании
		defer rf.Close()

		// создаём файл в архиве
		zf, e := z.Create(path)
		if e != nil {
			return fmt.Errorf("Error archive file: %w", e)
		}

		// копируем содержимое файла в архив
		_, e = io.Copy(zf, rf)
		if e != nil {
			return fmt.Errorf("Error copy file: %w", e)
		}
		// если всё хорошо - увеличим счётчик файлов
		count++
		return nil
	})

	if err != nil {
		return fmt.Errorf("Error create cbz file: %w", err)
	}

	// если всё хорошо - удаляем папку с файлами
	err = os.RemoveAll(d.title)
	if err != nil {
		return fmt.Errorf("Error remove original dir: %w", err)
	}

	// если внутри не было файлов - удаляем и архив
	if count == 0 {
		name := d.title + ".cbz"
		log.Printf("\nInfo: file %s has no files.\n", name)
		os.Remove(name)
	}

	fmt.Println()
	return nil
}
