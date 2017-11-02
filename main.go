package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bitly/go-simplejson"
	cors "github.com/heppu/simple-cors"
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
	Currency   string
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

	// Use the PORT environment variable
	port := os.Getenv("PORT")
	// Default to 3000 if no PORT environment variable was defined
	if port == "" {
		port = "8080"
	}
	//Routes
	router.GET("/welcome", handleWelcome)
	router.POST("/chat", handleChat)
	router.GET("/", handle)
	log.Fatal(http.ListenAndServe(":"+port, cors.CORS(router)))
}

// handle Handles /
func handle(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	body :=
		"<!DOCTYPE html><html><head><title>Chatbot</title></head><body><pre style=\"font-family: monospace;\">\n" +
			"Available Routes:\n\n" +
			"  GET  /welcome -> handleWelcome\n" +
			"  POST /chat    -> handleChat\n" +
			"  GET  /        -> handle        (current)\n" +
			"</pre></body></html>"
	w.Header().Add("Content-Type", "text/html")
	fmt.Fprintln(w, body)
}

func handleWelcome(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	hasher := sha256.New()
	hasher.Write([]byte(strconv.FormatInt(time.Now().Unix(), 10)))
	uuid := hex.EncodeToString(hasher.Sum(nil))

	// Create a session for this UUID
	sessions[uuid] = Session{}

	writeJSON(w, JSON{
		"message": "Welcome to The Luxury Shopper.<br> What are you looking for? say something like 'Gucci Tshirt' ",
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

	//Filter search by keyword
	returnValue := filterByCondition(session, message, w)
	if returnValue == 1 {
		return
	}
	returnValue2 := filterByMinPrice(session, message, w)
	if returnValue2 == 1 {
		return
	}
	returnValue3 := filterByMaxPrice(session, message, w)
	if returnValue3 == 1 {
		return
	}

	condition := session["condition"].(string)

	minPrice := session["minPrice"].(string)

	maxPrice := session["maxPrice"].(string)

	keyword := strings.Replace(session["searchByKeyword"].(string), " ", "%20", -1)

	numOfResults := strconv.Itoa(5)

	url := "http://svcs.ebay.com/services/search/FindingService/v1?OPERATION-NAME=findItemsByKeywords&SERVICE-VERSION=1.0.0&SECURITY-APPNAME=TheLuxur-TheLuxur-PRD-45d705b3d-83824180&RESPONSE-DATA-FORMAT=JSON&REST-PAYLOAD&paginationInput.entriesPerPage=" + numOfResults + "&keywords=" + keyword

	filterIndex := 0

	if !strings.EqualFold(session["condition"].(string), "none") {
		url += "&itemFilter(" + strconv.Itoa(filterIndex) + ").name=Condition&itemFilter(" + strconv.Itoa(filterIndex) + ").value=" + condition
		filterIndex++
	}

	if !strings.EqualFold(session["minPrice"].(string), "none") {
		url += "&itemFilter(" + strconv.Itoa(filterIndex) + ").name=MinPrice&itemFilter(" + strconv.Itoa(filterIndex) + ").value=" + minPrice
		filterIndex++
	}

	if !strings.EqualFold(session["maxPrice"].(string), "none") {
		url += "&itemFilter(" + strconv.Itoa(filterIndex) + ").name=MaxPrice&itemFilter(" + strconv.Itoa(filterIndex) + ").value=" + maxPrice
	}

	spaceClient := http.Client{
		Timeout: time.Second * 2, // Maximum of 2 secs
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

	// Handle Error
	handleError(js, session, w)

	//Handle the case where the number of items fetched is 0
	handleCaseZero(js, session, w)

	//Gerenate Response
	generateResponse(js, session, w, numOfResults)

}

//Helper methods

func filterByCondition(session Session, message string, w http.ResponseWriter) int {
	_, found1 := session["conditionBool"]
	if !found1 {
		session["conditionBool"] = false
	}
	_, found2 := session["condition"]
	if !found2 {
		//Respond with question about condition
		if !session["conditionBool"].(bool) {
			writeJSON(w, JSON{
				"message": "Please specify the condition of the required item. (New, Used or None)",
				"session": session,
			})
			session["conditionBool"] = true
			return 1
		} else {
			session["condition"] = message

			if strings.EqualFold(session["condition"].(string), "new") {
				session["condition"] = "New"
			} else if strings.EqualFold(session["condition"].(string), "used") {
				session["condition"] = "Used"
			}
		}
	}
	return 0
}

func filterByMinPrice(session Session, message string, w http.ResponseWriter) int {
	_, found3 := session["minPriceBool"]
	if !found3 {
		session["minPriceBool"] = false
	}
	_, found4 := session["minPrice"]
	if !found4 {
		//Respond with question about condition
		if !session["minPriceBool"].(bool) {
			writeJSON(w, JSON{
				"message": "Please specify the minimum price of the required item. (None in case you dont want to filter with minimum price)",
				"session": session,
			})
			session["minPriceBool"] = true
			return 1
		} else {
			session["minPrice"] = message
		}
	}
	return 0
}

func filterByMaxPrice(session Session, message string, w http.ResponseWriter) int {
	_, found5 := session["maxPriceBool"]
	if !found5 {
		session["maxPriceBool"] = false
	}
	_, found6 := session["maxPrice"]
	if !found6 {
		//Respond with question about condition
		if !session["maxPriceBool"].(bool) {
			writeJSON(w, JSON{
				"message": "Please specify the maximum price of the required item. (None in case you dont want to filter with maximum price)",
				"session": session,
			})
			session["maxPriceBool"] = true
			return 1
		} else {
			session["maxPrice"] = message
		}
	}
	return 0
}

func handleError(js *simplejson.Json, session Session, w http.ResponseWriter) {
	error, _ := js.Get("findItemsByKeywordsResponse").GetIndex(0).Get("ack").GetIndex(0).String()
	if strings.EqualFold(error, "failure") {
		errorMessage, _ := js.Get("findItemsByKeywordsResponse").GetIndex(0).Get("errorMessage").GetIndex(0).Get("error").GetIndex(0).Get("message").GetIndex(0).String()
		response := errorMessage + "<br>  What else would you like to search for? "
		writeJSON(w, JSON{
			"message": response,
		})
		for k := range session {
			delete(session, k)
		}
		return
	}
}

func handleCaseZero(js *simplejson.Json, session Session, w http.ResponseWriter) {
	itemCount, _ := js.Get("findItemsByKeywordsResponse").GetIndex(0).Get("searchResult").GetIndex(0).Get("@count").String()
	itemCount1, _ := strconv.Atoi(itemCount)
	if itemCount1 == 0 {
		response := "There are no items matching your criteria. <br> What else would you like to search for? "
		writeJSON(w, JSON{
			"message": response,
		})
		for k := range session {
			delete(session, k)
		}
		return
	}
}

func generateResponse(js *simplejson.Json, session Session, w http.ResponseWriter, numOfResults string) {
	simplifiedData1, _ := js.Get("findItemsByKeywordsResponse").GetIndex(0).Get("searchResult").GetIndex(0).Get("item").Array() // simplifiedData1 is the array of items fetched
	pageURL, _ := js.Get("findItemsByKeywordsResponse").GetIndex(0).Get("itemSearchURL").GetIndex(0).String()                   // ebay results page url

	//populate FetchedData struct
	var f FetchedData
	for _, element := range simplifiedData1 {
		element1 := element.(map[string]interface{})
		item1 := Item{ID: element1["itemId"].([]interface{})[0].(string),
			GalleryURL: element1["galleryURL"].([]interface{})[0].(string),
			ItemURL:    element1["viewItemURL"].([]interface{})[0].(string),
			Title:      element1["title"].([]interface{})[0].(string),
			Condition:  element1["condition"].([]interface{})[0].(map[string]interface{})["conditionDisplayName"].([]interface{})[0].(string),
			Price:      element1["sellingStatus"].([]interface{})[0].(map[string]interface{})["currentPrice"].([]interface{})[0].(map[string]interface{})["__value__"].(string),
			Currency:   element1["sellingStatus"].([]interface{})[0].(map[string]interface{})["currentPrice"].([]interface{})[0].(map[string]interface{})["@currencyId"].(string)}
		f.Items = append(f.Items, item1)
	}

	response := "There are " + numOfResults + " items matching your criteria : "
	for index, element := range f.Items {
		response += "<br> Item " + strconv.Itoa(index+1) + " Title : " + element.Title + "<br> Item " + strconv.Itoa(index+1) + " Condition : " + element.Condition
		response += "<br> Item " + strconv.Itoa(index+1) + " Price : " + element.Price + " " + element.Currency + "<br> Item " + strconv.Itoa(index+1) + " Gallery : <a href='" + element.GalleryURL + "'>" + element.ItemURL + "</a>"
		response += "<br> Item " + strconv.Itoa(index+1) + " URL : <a href='" + element.ItemURL + "'>" + element.ItemURL + "</a><br>"
	}
	response += "<br> Results Page URL : <a href='" + pageURL + "'>" + pageURL + "</a> <br>  What else would you like to search for?"
	writeJSON(w, JSON{
		"message": response,
	})
	for k := range session {
		delete(session, k)
	}
	return
}
