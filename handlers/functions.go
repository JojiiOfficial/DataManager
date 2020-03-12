package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"text/template"

	"github.com/JojiiOfficial/DataManagerServer/models"

	"github.com/JojiiOfficial/gaw"
	log "github.com/sirupsen/logrus"
)

func sendResponse(w http.ResponseWriter, status models.ResponseStatus, message string, payload interface{}, params ...int) {
	statusCode := http.StatusOK
	s := "0"
	if status == 1 {
		s = "1"
	}

	w.Header().Set(models.HeaderStatus, s)
	w.Header().Set(models.HeaderStatusMessage, message)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	if len(params) > 0 {
		statusCode = params[0]
		w.WriteHeader(statusCode)
	}

	var err error
	if payload != nil {
		err = json.NewEncoder(w).Encode(payload)
	} else if len(message) > 0 {
		_, err = fmt.Fprintln(w, message)
	}

	LogError(err)
}

func readRequestLimited(w http.ResponseWriter, r *http.Request, p interface{}, limit int64) bool {
	return readRequestBody(w, io.LimitReader(r.Body, limit), p)
}

//parseUserInput tries to read the body and parse it into p. Returns true on success
func readRequestBody(w http.ResponseWriter, r io.Reader, p interface{}) bool {
	body, err := ioutil.ReadAll(r)

	if LogError(err) {
		return false
	}

	return !handleAndSendError(json.Unmarshal(body, p), w, models.WrongInputFormatError, http.StatusUnprocessableEntity)
}

func handleAndSendError(err error, w http.ResponseWriter, message string, statusCode int) bool {
	if !LogError(err) {
		return false
	}
	sendResponse(w, models.ResponseError, message, nil, statusCode)
	return true
}

func sendServerError(w http.ResponseWriter) {
	sendResponse(w, models.ResponseError, "internal server error", nil, http.StatusInternalServerError)
}

//LogError returns true on error
func LogError(err error, context ...map[string]interface{}) bool {
	if err == nil {
		return false
	}

	if len(context) > 0 {
		log.WithFields(context[0]).Error(err.Error())
	} else {
		log.Error(err.Error())
	}
	return true
}

//AllowedSchemes schemes that are allowed in urls
var AllowedSchemes = []string{"http", "https"}

func isValidHTTPURL(inp string) bool {
	//check for valid URL
	u, err := url.Parse(inp)
	if err != nil {
		return false
	}

	return gaw.IsInStringArray(u.Scheme, AllowedSchemes)
}

func isStructInvalid(x interface{}) bool {
	s := reflect.TypeOf(x)
	for i := s.NumField() - 1; i >= 0; i-- {
		e := reflect.ValueOf(x).Field(i)

		if hasEmptyValue(e) {
			return true
		}
	}
	return false
}

func hasEmptyValue(e reflect.Value) bool {
	switch e.Type().Kind() {
	case reflect.String:
		if e.String() == "" || strings.Trim(e.String(), " ") == "" {
			return true
		}
	case reflect.Array:
		for j := e.Len() - 1; j >= 0; j-- {
			isEmpty := hasEmptyValue(e.Index(j))
			if isEmpty {
				return true
			}
		}
	case reflect.Slice:
		return isStructInvalid(e)

	case
		reflect.Uintptr, reflect.Ptr, reflect.UnsafePointer,
		reflect.Uint64, reflect.Uint, reflect.Uint8, reflect.Bool,
		reflect.Struct, reflect.Int64, reflect.Int:
		{
			return false
		}
	default:
		log.Error(e.Type().Kind(), e)
		return true
	}
	return false
}

func downloadHTTP(user *models.User, url string, f *os.File, file *models.File) (int, error) {
	res, err := http.Get(url)
	if LogError(err) {
		return 0, err
	}

	//Don't read content on http error
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, nil
	}

	//Check if file is too large
	if user.HasUploadLimit() && res.ContentLength > user.Role.MaxURLcontentSize {
		return res.StatusCode, errors.New("File too large")
	}

	//read response
	var reader io.Reader
	if user.HasUploadLimit() {
		//Use limited reader if user has limited download content size
		reader = io.LimitReader(res.Body, user.Role.MaxURLcontentSize)
	} else {
		//use body as reader to read everything
		reader = res.Body
	}

	//Save body in file
	size, err := io.Copy(f, reader)
	if LogError(err) {
		return 0, err
	}

	if err = res.Body.Close(); LogError(err) {
		return 0, err
	}

	//Set file size
	file.FileSize = size
	return res.StatusCode, nil
}

//BufferedCopy copies stream buffered. Buffersize in bytes
func BufferedCopy(bufferSize int, writer io.Writer, reader io.Reader) (err error) {
	buf := make([]byte, bufferSize)
	var n int

	for {
		n, err = reader.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}

		if n == 0 {
			break
		}

		if _, err = writer.Write(buf[:n]); err != nil {
			return err
		}
	}

	return
}

//Copy stream
func serveFileStream(config *models.Config, reader io.Reader, w http.ResponseWriter) error {
	err := gaw.BufferedCopy(config.Webserver.DownloadFileBuffer, w, reader)
	//Ignore EOF
	if err == io.EOF {
		return nil
	}
	return err
}

//Detect and set Content-Type by extension
func autoSetContentType(w http.ResponseWriter, file string) {
	setContentType(w, mime.TypeByExtension(file))
}

//Set Content-Type
func setContentType(w http.ResponseWriter, contentType string) {
	w.Header().Set(models.HeaderContentType, fmt.Sprintf("%s; charset=utf-8", contentType))
}

//Serve static file
func serveStaticFile(config *models.Config, file string, w http.ResponseWriter, contentType ...string) error {
	//Open file
	f, err := os.Open(config.GetHTMLFile(file))
	defer f.Close()

	if LogError(err) {
		return err
	}

	//Set contenttype
	if len(contentType) == 0 || len(contentType[0]) == 0 {
		autoSetContentType(w, file)
	} else {
		w.Header().Set(models.HeaderContentType, contentType[0])
	}

	return serveFileStream(config, f, w)
}

func serveTemplate(config *models.Config, file string, w http.ResponseWriter, data interface{}) error {
	//Read file
	fileContent, err := ioutil.ReadFile(config.GetHTMLFile(file))
	if err != nil {
		return err
	}

	//Create template
	t := template.New("template")
	t, err = t.Parse(string(fileContent))
	if err != nil {
		return err
	}

	return t.Execute(w, data)
}

//Handles errors and respond with 404 if this caused the error
func handleBrowserServeError(err error, handerData handlerData, w http.ResponseWriter, r *http.Request) {
	if err != nil {
		fmt.Println(err)
		if os.IsNotExist(err) {
			NotFoundHandler(handerData, w, r)
			return
		}
		http.Error(w, "Server error", http.StatusInternalServerError)
	}
}

func returnRawByUseragent(useragent string) bool {
	useragent = strings.ToLower(useragent)
	return strings.HasPrefix(useragent, "curl") || strings.HasPrefix(useragent, "wget")
}
