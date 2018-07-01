package main

// === DECLS

import (
	"net/http"
	"math/rand"
	"time"
	"errors"
	"encoding/json"
	"log"
	"io/ioutil"
)

type (

	Config struct {
		Secret string `json:"secret"`
	}

	MemoryMessageStore struct {
		messages *[]Message
	}

	Message struct {
		Contents interface{} `json:"contents"`
	}

	AuthMessage struct {
		Message  `json:"message"`
		Secret string `json:"secret"`
	}

	MessageStore interface {
		GetMessage()(Message, error)
		StoreMessage(Message)(error)
	}

	ReturnStatus struct {
		Success bool `json:"success"`
		Context string `json:"context"`
	}

	ReturnMessage struct {
		Message Message `json:"message"`
		Status ReturnStatus `json:"status"`
	}

)

var (
	authSecret string
	DefaultAuthStore    = MemoryMessageStore{&[]Message{}}
	DefaultNonAuthStore = MemoryMessageStore{&[]Message{}}
)

// === UTILS

func seedRandom() {rand.Seed(time.Now().Unix())}

// === METHODS

func (mms MemoryMessageStore) GetMessage() (Message, error) {

	var messageCount = len(*mms.messages)

	// If nothing's there yet, return nothing.
	if messageCount == 0 {return Message{}, errors.New("no message available")}

	// Something's there, so return a random element from the slice.
	return (*mms.messages)[ rand.Intn(messageCount) ], nil
}

func (mms MemoryMessageStore) StoreMessage(message Message) (error) {
	*mms.messages = append(*mms.messages, message)
	return nil
}


// === CORE

// ========  Response writers

func returnEncodingError(resp http.ResponseWriter) () {
	writeJSONResponse("Bad encoding of input json", false, resp)
}

func returnBadSecretError(resp http.ResponseWriter) () {
	writeJSONResponse("Bad secret passed to authorise messages", false, resp)
}

func returnStorageError(resp http.ResponseWriter) () {
	writeJSONResponse("Could not store provided message internally", false, resp)
}

func returnNoAvailableMessageError(resp http.ResponseWriter) () {
	writeJSONResponse("No message available", false, resp)
}

func returnReceiveSuccess(resp http.ResponseWriter) () {
	writeJSONResponse("Message successfully stored", true, resp)
}

func writeJSONResponse(context string, success bool, resp http.ResponseWriter) () {
	encodingErrorMessage := ReturnStatus{Success:success, Context:context}
	errJson, marshallingErr := json.Marshal(encodingErrorMessage)

	if marshallingErr != nil {
		//  THIS SHOULD NEVER HAPPEN so I think panicking is appropriate. Maybe handle better in future.
		panic(marshallingErr)
	}

	resp.Write(errJson)
}


func writeAsJSON(toWrite interface{}, resp http.ResponseWriter) {
	messageJSON, err := json.Marshal(toWrite)

	if err != nil {
		//  THIS SHOULD NEVER HAPPEN so I think panicking is appropriate. Maybe handle better in future.
		panic(err)
	}

	resp.Write(messageJSON)

}



// ========  JSON handlers
func SimpleMessageHandler(recieveStore, sendStore MessageStore, postingAuthRequired bool) (func(http.ResponseWriter, *http.Request)) {

	return func(w http.ResponseWriter, r *http.Request) {

		print("Processing request")

		switch r.Method {

		case "GET":
			print("Processing 'get'")
			seedRandom()
			message, err := sendStore.GetMessage()  // NOTE: we're only sending *to be authorised* messages, i.e. they are *not* authorised but will be *judged for authorisation* by this process.
			if err != nil {
				returnNoAvailableMessageError(w)
				return
			}
			writeAsJSON(message, w)

		case "POST":
			print("Processing 'post'")
			// Decode the json message from the body
			newMessage := &AuthMessage{} // Messages *must* be authorised!
			d := json.NewDecoder(r.Body)
			encodingError := d.Decode(newMessage)
			authProvided := newMessage.Secret
			messageProvided := newMessage.Message

			// If we hit an error, write that error /and then return out of this function early./
			// We're done if we can't encode the message.
			if encodingError != nil {
				returnEncodingError(w)
				return
			}

			// Check for authorisation if required
			if postingAuthRequired {
				if authProvided != authSecret {
					returnBadSecretError(w)
					return
				}
			}

			// Actually store the message, and write a message depending on the success.
			err := recieveStore.StoreMessage(messageProvided)  // NOTE: we store this as non-authorised for now!
			if err != nil {
				returnStorageError(w)
				return

				// If everything's gone well, write a response message and let's get outta here.
			} else {
				returnReceiveSuccess(w)
				return
			}

			println("done")

		}

	}

}

func AuthorisedMessageHandler(recieveStore, sendStore MessageStore) (func(http.ResponseWriter, *http.Request)) {
	return SimpleMessageHandler(recieveStore, sendStore, true)
}

func NonAuthorisedMessageHandler(recieveStore, sendStore MessageStore) (func(http.ResponseWriter, *http.Request)) {
	return SimpleMessageHandler(recieveStore, sendStore, false)
}


// === Main methods

func ConfigureMerlin()() {

	// Get the authorisation secret from config. If it doesn't exist, *abort*.
	confData, err := ioutil.ReadFile("config.json")
	if err != nil {panic("Missing configuration! Could not get admin secret; aborting rather than running insecurely.")}

	conf := &Config{}
	err = json.Unmarshal(confData, conf)

	if err != nil || conf.Secret == "" {

		panic("Bad configuration! Could not get admin secret; aborting rather than running insecurely.")

	} else {

		authSecret = conf.Secret

	}

}

func Serve(nonAuthStore, authStore MessageStore) {

	// TODO: re-enable this so `nil` can be passed into Serve in the main function
	//if nonAuthStore == nil {
	//
	//	nonAuthStore = MessageStore(DefaultNonAuthStore)
	//}
	//
	//if authStore == nil {
	//	authStore = MessageStore(DefaultAuthStore)
	//}

	// Spin the http server

	http.HandleFunc("/", NonAuthorisedMessageHandler(nonAuthStore, authStore))
	http.HandleFunc("/admin", AuthorisedMessageHandler(authStore, nonAuthStore)) // TODO: make "admin" token configurable
	log.Fatal(http.ListenAndServe(":8080", nil))

}


func main() {
	ConfigureMerlin()
	Serve(DefaultNonAuthStore, DefaultAuthStore)  // Serve with default message stores, TODO: make this configurable by config file
}
