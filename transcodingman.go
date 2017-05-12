package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/labstack/gommon/log"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type (
	Response struct {
		Message string      `json:"message"` //encode result message
		Source  string      `json:"source"`  //encode video source
		URL     string      `json:"url"`     //encode video url
		Detail  interface{} `json:"detail"`  //encode video detail infomation
	}

	Extension struct {
		FullPath  string
		Path      string
		Extension string
		Name      string
	}

	Err struct {
		Code    string `json:"code"`
		Step    string `json:"step"`
		Message string `json:"message"`
	}
)

var (
	ffmpeg  string
	ffprobe string
)

const (
	incomingPath string = "incoming/"
	outgoingPath string = "outgoing/"
)

func init() {
	ffmpeg, _ = exec.LookPath("ffmpeg")
	ffprobe, _ = exec.LookPath("ffprobe")
}

func main() {
	if ffmpeg == "" {
		panic("ffmpeg was not found")
	}
	if ffprobe == "" {
		panic("ffprobe was not found")
	}

	//init server
	e := echo.New()
	// Routing
	e.GET("/transcoding", transcoding)
	e.Static("/", "static")
	// Root level middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())

	e.Logger.Fatal(e.Start(":5000"))
}

func transcoding(c echo.Context) error {
	source := c.QueryParam("source")
	options := c.QueryParam("options")
	finfo, err := getExtension(source)
	if err != nil {
		return printError("001", "get extension", err.Error(), c)
	}

	cmdInput := source
	if finfo.Extension == "zip" {
		unzipPath := finfo.FullPath[:strings.LastIndex(finfo.FullPath, ".zip")]
		cmdInput = unzipPath + "/%d.jpg"
		err := unzip(finfo.FullPath, unzipPath)
		if err != nil {
			clearFile(finfo)
			return printError("002", "unzip", err.Error(), c)
		}
	}

	if len(options) < 2 {
		options = "-s 160x64 -c:v mjpeg -q:v 5 -r 20 -an"
	}

	outPath := outgoingPath + time.Now().Format("20060102")
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		os.MkdirAll(outPath, 0777)
	}

	var errOut bytes.Buffer
	var out bytes.Buffer

	cmdOutput := outPath + "/" + time.Now().Format("20060102150405") + "_" + finfo.Name + ".avi"
	cmdOption := "-i " + cmdInput + " " + options + " -v 16 " + cmdOutput
	cmd := exec.Command(ffmpeg, strings.Fields(cmdOption)...)
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		clearFile(finfo)
		message := strings.Replace(errOut.String(), incomingPath, "", -1)
		return printError("003", "ffmpeg", message, c)
	}

	infoCmd := exec.Command(ffprobe, "-show_streams", "-of", "json", cmdOutput)
	infoCmd.Stderr = &errOut
	infoCmd.Stdout = &out
	if err := infoCmd.Run(); err != nil {
		clearFile(finfo)
		message := strings.Replace(errOut.String(), incomingPath, "", -1)
		return printError("004", "ffprobe", message, c)
	}

	result := &Response{Message: "complete",
		URL:    cmdOutput[len(outgoingPath):],
		Source: source}
	var detail map[string]interface{}
	json.Unmarshal(out.Bytes(), &detail)
	result.Detail = detail["streams"].([]interface{})[0]

	if finfo.Extension == "zip" {
		unzipPath := finfo.FullPath[:strings.LastIndex(finfo.FullPath, ".zip")]
		if err := os.RemoveAll(unzipPath); err != nil {
			printError("005", "origin file remove", err.Error(), nil)
		}
		mvCmd := exec.Command("mv", finfo.FullPath, cmdOutput+".origin.zip")
		if err := mvCmd.Run(); err != nil {
			printError("006", "origin file rename", err.Error(), nil)
		}

	}
	return c.JSON(200, result)
}

func clearFile(info Extension) {
	os.RemoveAll(info.FullPath)
	os.RemoveAll(info.FullPath[:strings.LastIndex(info.FullPath, ".zip")])
}

func printError(code, step, message string, c echo.Context) error {
	errorMap := make(map[string]interface{})
	errorMap["code"] = code
	errorMap["step"] = step
	errorMap["message"] = message
	log.Errorj(errorMap)
	if c != nil {
		return echo.NewHTTPError(500, errorMap)
	} else {
		return nil
	}
}

func getExtension(input string) (Extension, error) {
	result := Extension{}
	result.FullPath = incomingPath + input
	if _, err := os.Stat(result.FullPath); os.IsNotExist(err) {
		return result, errors.New(input + " is not exist.")
	}
	if strings.LastIndex(result.FullPath, ".") < 2 {
		return result, errors.New(input + " has not extension.")
	}
	result.Path = input[:strings.LastIndex(input, "/")+1]
	result.Extension = input[strings.LastIndex(input, ".")+1:]
	result.Name = input[strings.LastIndex(input, "/")+1 : strings.LastIndex(input, ".")]
	return result, nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	os.MkdirAll(dest, 0755)

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}
