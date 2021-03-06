package faunadb

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	defaultEndpoint = "https://db.fauna.com"
	requestTimeout  = 60 * time.Second
)

var resource = ObjKey("resource")

// ClientConfig are used to apply specific configurations to the FaunaClient structure.
type ClientConfig func(*FaunaClient)

// Endpoint configures the FaunaClient structure to send requests to a specific FaunaDB url.
func Endpoint(url string) ClientConfig { return func(cli *FaunaClient) { cli.endpoint = url } }

// HTTP configures the FaunaClient structure to use a specific http.Client.
func HTTP(http *http.Client) ClientConfig { return func(cli *FaunaClient) { cli.http = http } }

/*
FaunaClient provides methods for performing queries on a FaunaDB cluster.

This structure should be reused as much as possible. Avoid copying this structure.
If you need to create a client with a different secret, use the NewSessionClient method.
*/
type FaunaClient struct {
	basicAuth string
	endpoint  string
	http      *http.Client
}

/*
NewFaunaClient creates a new FaunaClient structure. Possible configurations are:
	Endpoint: sets a specific FaunaDB url. Default: https://db.fauna.com
		HTTP: sets a specific http.Client. Default: a new net.Client with 60 seconds timeout.
*/
func NewFaunaClient(secret string, configs ...ClientConfig) *FaunaClient {
	client := &FaunaClient{basicAuth: basicAuth(secret)}

	for _, config := range configs {
		config(client)
	}

	if client.endpoint == "" {
		client.endpoint = defaultEndpoint
	}

	if client.http == nil {
		client.http = &http.Client{
			Timeout: requestTimeout,
		}
	}

	return client
}

// Query sends a query language expression to FaunaDB
func (client *FaunaClient) Query(expr Expr) (value Value, err error) {
	response, err := client.performRequest(expr)

	if response != nil {
		defer func() {
			_, _ = io.Copy(ioutil.Discard, response.Body) // Discard remaining bytes so the connection can be reused
			_ = response.Body.Close()
		}()
	}

	if err == nil {
		if err = checkForResponseErrors(response); err == nil {
			value, err = client.parseResponse(response)
		}
	}

	return
}

// BatchQuery sends multiple query language expressions to FaunaDB
func (client *FaunaClient) BatchQuery(exprs []Expr) (values []Value, err error) {
	arr := make(unescapedArr, len(exprs))

	for i, expr := range exprs {
		arr[i] = expr
	}

	var res Value

	if res, err = client.Query(arr); err == nil {
		err = res.Get(&values)
	}

	return
}

// NewSessionClient creates a new child FaunaClient with the specified secret. The new client reuses its parents internal http resources.
func (client *FaunaClient) NewSessionClient(secret string) *FaunaClient {
	return &FaunaClient{
		basicAuth: basicAuth(secret),
		endpoint:  client.endpoint,
		http:      client.http,
	}
}

func (client *FaunaClient) performRequest(expr Expr) (response *http.Response, err error) {
	var request *http.Request

	if request, err = client.prepareRequest(expr); err == nil {
		response, err = client.http.Do(request)
	}

	return
}

func (client *FaunaClient) prepareRequest(expr Expr) (request *http.Request, err error) {
	var body []byte

	if body, err = json.Marshal(expr); err == nil {
		if request, err = http.NewRequest("POST", client.endpoint, bytes.NewReader(body)); err == nil {
			request.Header.Add("Authorization", client.basicAuth)
			request.Header.Add("Content-Type", "application/json; charset=utf-8")
		}
	}

	return
}

func (client *FaunaClient) parseResponse(response *http.Response) (Value, error) {
	value, err := parseJSON(response.Body)

	if err != nil {
		return nil, err
	}

	return value.At(resource).GetValue()
}

func basicAuth(secret string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(secret))
	return fmt.Sprintf("Basic %s:", encoded)
}
