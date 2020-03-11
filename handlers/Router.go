package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/JojiiOfficial/DataManagerServer/models"
	"github.com/JojiiOfficial/gaw"
	"github.com/jinzhu/gorm"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type handlerData struct {
	config *models.Config
	db     *gorm.DB
	user   *models.User
}

//Route for REST
type Route struct {
	Name        string
	Method      HTTPMethod
	Pattern     string
	HandlerFunc RouteFunction
	HandlerType requestType
}

//HTTPMethod http method. GET, POST, DELETE, HEADER, etc...
type HTTPMethod string

//HTTP methods
const (
	GetMethod    HTTPMethod = "GET"
	POSTMethod   HTTPMethod = "POST"
	DeleteMethod HTTPMethod = "DELETE"
)

type requestType uint8

const (
	defaultRequest requestType = iota
	sessionRequest
	optionalTokenRequest
)

//Routes all REST routes
type Routes []Route

//RouteFunction function for handling a route
type RouteFunction func(handlerData, http.ResponseWriter, *http.Request)

//Routes
var (
	routes = Routes{
		// Ping
		Route{
			Name:        "ping",
			Pattern:     "/ping",
			Method:      POSTMethod,
			HandlerFunc: Ping,
			HandlerType: defaultRequest,
		},
		// User
		Route{
			Name:        "login",
			Pattern:     "/user/login",
			Method:      POSTMethod,
			HandlerFunc: Login,
			HandlerType: defaultRequest,
		},
		Route{
			Name:        "register",
			Pattern:     "/user/register",
			Method:      POSTMethod,
			HandlerFunc: Register,
			HandlerType: defaultRequest,
		},

		// Files
		Route{
			Name:        "upload",
			Pattern:     "/upload/file",
			Method:      POSTMethod,
			HandlerFunc: UploadfileHandler,
			HandlerType: sessionRequest,
		},
		Route{
			Name:        "list files",
			Pattern:     "/files",
			Method:      POSTMethod,
			HandlerFunc: ListFilesHandler,
			HandlerType: sessionRequest,
		},
		Route{
			Name:        "update file",
			Pattern:     "/file/{action}",
			Method:      POSTMethod,
			HandlerFunc: FileHandler,
			HandlerType: sessionRequest,
		},

		// Preview
		Route{
			Name:        "preview",
			Pattern:     "/preview/{fileID}",
			HandlerFunc: PrevievFileHandler,
			HandlerType: defaultRequest,
			Method:      GetMethod,
		},

		// Attribute
		Route{
			Name:        "Attribute",
			Pattern:     "/attribute/{attribute}/{action}",
			Method:      POSTMethod,
			HandlerFunc: AttributeHandler,
			HandlerType: sessionRequest,
		},

		//Namespace
		Route{
			Name:        "Namespace",
			Pattern:     "/namespace/{action}",
			Method:      POSTMethod,
			HandlerFunc: NamespaceActionHandler,
			HandlerType: sessionRequest,
		},
		Route{
			Name:        "Namespace list",
			Pattern:     "/namespaces",
			Method:      POSTMethod,
			HandlerFunc: NamespaceListHandler,
			HandlerType: sessionRequest,
		},
	}
)

//NewRouter create new router
func NewRouter(config *models.Config, db *gorm.DB) *mux.Router {
	handlerData := handlerData{
		config: config,
		db:     db,
	}

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		router.
			Methods(string(route.Method)).
			Path(route.Pattern).
			Name(route.Name).
			Handler(RouteHandler(route.HandlerType, &handlerData, route.HandlerFunc, route.Name))
	}

	//Adding custom routes
	addCustomRoutes(router, &handlerData)

	return router
}

func addCustomRoutes(router *mux.Router, handlerData *handlerData) {
	// 404 Handler
	router.NotFoundHandler = RouteHandler(defaultRequest, handlerData, NotFoundHandler, "not found")

	// Index routes
	handler := RouteHandler(defaultRequest, handlerData, IndexPageHandler, "index")
	router.Handle("/", handler)
	router.Handle("/preview/", handler)

	// Serve static files
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./html/static"))))
}

//RouteHandler logs stuff
func RouteHandler(requestType requestType, handlerData *handlerData, inner RouteFunction, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[%s] %s\n", r.Method, name)

		start := time.Now()

		if validateHeader(handlerData.config, w, r) {
			return
		}

		//Validate request by requestType
		if !requestType.validate(handlerData, r, w) {
			return
		}

		//Process request
		inner(*handlerData, w, r)

		//Print duration of processing
		printProcessingDuration(start)
	})
}

//Return false on error
func (requestType requestType) validate(handlerData *handlerData, r *http.Request, w http.ResponseWriter) bool {
	switch requestType {
	case sessionRequest:
		{
			authHandler := NewAuthHandler(r)
			if len(authHandler.GetBearer()) != 64 {
				sendResponse(w, models.ResponseError, "Invalid token", http.StatusUnauthorized)
				return false
			}

			user, err := models.GetUserFromSession(handlerData.db, authHandler.GetBearer())
			if err != nil || user == nil {
				sendResponse(w, models.ResponseError, "Invalid token", http.StatusUnauthorized)
				return false
			}

			handlerData.user = user
		}
	}

	return true
}

//Prints the duration of handling the function
func printProcessingDuration(startTime time.Time) {
	dur := time.Since(startTime)

	if dur < 1500*time.Millisecond {
		log.Debugf("Duration: %s\n", dur.String())
	} else if dur > 1500*time.Millisecond {
		log.Warningf("Duration: %s\n", dur.String())
	}
}

//Return true on error
func validateHeader(config *models.Config, w http.ResponseWriter, r *http.Request) bool {
	headerSize := gaw.GetHeaderSize(r.Header)

	//Send error if header are too big. MaxHeaderLength is stored in b
	if headerSize > uint32(config.Webserver.MaxHeaderLength) {
		//Send error response
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		fmt.Fprint(w, "413 request too large")

		log.Warnf("Got request with %db headers. Maximum allowed are %db\n", headerSize, config.Webserver.MaxHeaderLength)
		return true
	}

	return false
}
