package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bitly/go-simplejson"
	"github.com/julienschmidt/httprouter"
)

type FetchedData struct {
	Items []Item
}

type Item struct {
	ID         string
	GalleryURL string
	ItemURL    string
	Title      string
	Condition  string
	Price      string
}

var (
	sessions  = map[string]Session{}
	processor = sampleProcessor
)

type (
	// Session Holds info about a session
	Session map[string]interface{}

	// JSON Holds a JSON object
	JSON map[string]interface{}

	// Processor Alias for Process func
	Processor func(session Session, message string, w http.ResponseWriter)
)

func main() {
	//Initialize http router
	router := httprouter.New()

	//Routes
	router.GET("/welcome", handleWelcome)
	router.POST("/chat", handleChat)
	log.Fatal(http.ListenAndServe(":8080", router))
}

func handleWelcome(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	hasher := sha256.New()
	hasher.Write([]byte(strconv.FormatInt(time.Now().Unix(), 10)))
	uuid := hex.EncodeToString(hasher.Sum(nil))

	// Create a session for this UUID
	sessions[uuid] = Session{}

	writeJSON(w, JSON{
		"message": "Welcome to The Luxury Shopper. Are you looking for anything in particular? say something like 'Gucci Tshirt' or 'no' ",
		"uuid":    uuid,
	})
}

func handleChat(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	// Make sure only POST requests are handled
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST requests are allowed.", http.StatusMethodNotAllowed)
		return
	}

	// Make sure a UUID exists in the Authorization header
	uuid := r.Header.Get("Authorization")
	if uuid == "" {
		http.Error(w, "Missing or empty Authorization header.", http.StatusUnauthorized)
		return
	}

	// Make sure a session exists for the extracted UUID
	session, sessionFound := sessions[uuid]
	if !sessionFound {
		http.Error(w, fmt.Sprintf("No session found for: %v.", uuid), http.StatusUnauthorized)
		return
	}

	// Parse the JSON string in the body of the request
	data := JSON{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, fmt.Sprintf("Couldn't decode JSON: %v.", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Make sure a message key is defined in the body of the request
	_, messageFound := data["message"]
	if !messageFound {
		http.Error(w, "Missing message key in body.", http.StatusBadRequest)
		return
	}

	processor(session, data["message"].(string), w)
}

// writeJSON Writes the JSON equivilant for data into ResponseWriter w
func writeJSON(w http.ResponseWriter, data JSON) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// ProcessFunc Sets the processor of the chatbot
func ProcessFunc(p Processor) {
	processor = p
}

func sampleProcessor(session Session, message string, w http.ResponseWriter) {
	//Check if there is already an existing value assigned to searchByKeyword in this session
	_, found := session["searchByKeyword"]
	if !found {
		session["searchByKeyword"] = message
	}
	//If the value assigned to searchByKeyword is no then the response is the next question otherwise search by the keyword given by the user
	if session["searchByKeyword"].(string) != "no" {
		message1 := strings.Replace(session["searchByKeyword"].(string), " ", "%20", -1)
		numOfResults := strconv.Itoa(2)
		url := "http://svcs.ebay.com/services/search/FindingService/v1?OPERATION-NAME=findItemsByKeywords&SERVICE-VERSION=1.0.0&SECURITY-APPNAME=TheLuxur-TheLuxur-PRD-45d705b3d-83824180&RESPONSE-DATA-FORMAT=JSON&REST-PAYLOAD&paginationInput.entriesPerPage=" + numOfResults + "&keywords=" + message1

		spaceClient := http.Client{
			Timeout: time.Second * 300, // Maximum of 2 secs
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			log.Fatal(err)
		}

		res, getErr := spaceClient.Do(req)
		if getErr != nil {
			log.Fatal(getErr)
		}

		body, readErr := ioutil.ReadAll(res.Body)
		if readErr != nil {
			log.Fatal(readErr)
		}

		js, jsonErr := simplejson.NewJson([]byte(body))
		if jsonErr != nil {
			log.Fatal(jsonErr)
		}
		simplifiedData1, _ := js.Get("findItemsByKeywordsResponse").GetIndex(0).Get("searchResult").GetIndex(0).Get("item").Array() // simplifiedData1 is the array of items fetched

		//populate FetchedData struct
		var f FetchedData

		for _, element := range simplifiedData1 {
			element1 := element.(map[string]interface{})
			fmt.Println(element1["itemId"].([]interface{})[0])
			item1 := Item{ID: element1["itemId"].([]interface{})[0].(string),
				GalleryURL: element1["galleryURL"].([]interface{})[0].(string),
				ItemURL:    element1["viewItemURL"].([]interface{})[0].(string),
				Title:      element1["title"].([]interface{})[0].(string),
				Condition:  element1["condition"].([]interface{})[0].(map[string]interface{})["conditionDisplayName"].([]interface{})[0].(string),
				Price:      element1["sellingStatus"].([]interface{})[0].(map[string]interface{})["currentPrice"].([]interface{})[0].(map[string]interface{})["__value__"].(string)}
			f.Items = append(f.Items, item1)
		}

		//Respond with the array of items
		writeJSON(w, JSON{
			"message": f,
		})
		return
	} else {
		//Next question
		writeJSON(w, JSON{
			"message": "What are u looking for?",
		})
		return
	}

}
