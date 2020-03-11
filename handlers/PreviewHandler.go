package handlers

import (
	"net/http"
	"os"

	"github.com/JojiiOfficial/DataManagerServer/models"
	"github.com/JojiiOfficial/gaw"
	"github.com/gorilla/mux"
)

//PrevievFileHandler handler for previews
func PrevievFileHandler(handlerData handlerData, w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	fileID, _ := vars["fileID"]

	//search file
	file, found, err := models.GetPublicFile(handlerData.db, fileID)
	if !found {
		http.NotFound(w, r)
		return
	}

	//Send error
	if LogError(err) {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	//Send not found if not public
	if !file.IsPublic {
		http.NotFound(w, r)
		return
	}

	//Set content type header if available
	if len(file.FileType) > 0 {
		w.Header().Set("Content-Type", file.FileType)
	}

	//Open file
	f, err := os.Open(handlerData.config.GetStorageFile(file.LocalName))
	if LogError(err) {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	//Copy stream
	gaw.BufferedCopy(handlerData.config.Webserver.DownloadFileBuffer, w, f)

	if LogError(err) {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	//Close file
	LogError(f.Close())
}

//IndexPageHandler show index/main page
func IndexPageHandler(handlerData handlerData, w http.ResponseWriter, r *http.Request) {
	// Print main page
	// TODO
}