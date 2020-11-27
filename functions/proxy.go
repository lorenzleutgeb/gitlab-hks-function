package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/oauth2"
	"gopkg.in/hockeypuck/hkp.v1"
	"gopkg.in/hockeypuck/openpgp.v1"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

var gitlabClient *http.Client

var mrFormat hkp.IndexFormat
var jsonFormat hkp.IndexFormat

var gitlabHost string

func init() {
	mrFormat = &hkp.MRFormat{}
	jsonFormat = &hkp.JSONFormat{}

	gitlabHost = os.Getenv("GITLAB_HOST")

	transport := &oauth2.Transport{
		Source: oauth2.StaticTokenSource(
			&oauth2.Token{
				AccessToken: os.Getenv("GITLAB_TOKEN"),
			},
		),
	}

	gitlabClient = &http.Client{
		Timeout:   time.Second * 4,
		Transport: transport,
	}
}

func base() *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   gitlabHost,
		Path:   path.Join("api", "v4"),
	}
}

func keyURL(userID int) *url.URL {
	result := base()
	result.Path = path.Join(result.Path, "users", strconv.Itoa(userID), "gpg_keys")
	return result
}

func searchURL(search string) *url.URL {
	result := base()
	result.Path = path.Join(result.Path, "users")
	query := result.Query()
	query.Set("search", search)
	result.RawQuery = query.Encode()
	return result
}

func Keyserver(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/pks/add" {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
		return
	}

	if r.URL.Path != "/pks/lookup" {
		http.Error(w, "Not Found", http.StatusNotFound)
	}

	l, err := hkp.ParseLookup(r)
	if err != nil {
		log.Printf("lookup: %v", l)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch l.Op {
	case hkp.OperationGet, hkp.OperationHGet:
		get(w, l)
	case hkp.OperationIndex:
		index(w, l)
	case hkp.OperationVIndex:
		index(w, l)
	case hkp.OperationStats:
		http.Error(w, "Operation not implemented", http.StatusNotImplemented)
	default:
		http.Error(w, fmt.Sprintf("operation not found: %v", l.Op), http.StatusBadRequest)
		return
	}
}

func index(w http.ResponseWriter, l *hkp.Lookup) {
	keys, err := keys(l)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(keys) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	f := mrFormat
	if l.Options[hkp.OptionJSON] {
		f = jsonFormat
	}

	err = f.Write(w, l, keys)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type user struct {
	ID       int
	Name     string
	Username string
	WebURL   string `json:"web_url"`
}

type key struct {
	Id        int
	Key       string
	CreatedAt time.Time `json:"created_at"`
}

func keys(lookup *hkp.Lookup) ([]*openpgp.PrimaryKey, error) {
	search := searchURL(lookup.Search).String()
	log.Printf("search: %v", search)
	res, err := gitlabClient.Get(search)
	if err != nil {
		return nil, err
	}

	users := make([]user, 0)
	err = json.NewDecoder(res.Body).Decode(&users)
	if err != nil {
		return nil, err
	}

	if len(users) != 1 {
		return nil, errors.New(fmt.Sprintf("expected exactly 1 user to match, but got %v: %v", len(users), users))
	}

	keys2 := keyURL(users[0].ID).String()
	log.Printf("keys: %v", keys2)
	res, err = gitlabClient.Get(keys2)
	if err != nil {
		return nil, err
	}

	keys := make([]key, 0)
	err = json.NewDecoder(res.Body).Decode(&keys)
	if err != nil {
		return nil, err
	}

	result := make([]*openpgp.PrimaryKey, 0)
	for _, key := range keys {
		primaryKeys, err := unarmor(key.Key)
		if err != nil {
			return nil, err
		}

		result = append(result, primaryKeys...)
	}

	return result, nil
}

func unarmor(armored string) ([]*openpgp.PrimaryKey, error) {
	armorBlock, err := armor.Decode(strings.NewReader(armored))
	if err != nil {
		return nil, err
	}

	result := make([]*openpgp.PrimaryKey, 0)

	for readKey := range openpgp.ReadKeys(armorBlock.Body) {
		if readKey.Error != nil {
			return nil, readKey.Error
		}
		err := openpgp.DropDuplicates(readKey.PrimaryKey)
		if err != nil {
			return nil, err
		}

		result = append(result, readKey.PrimaryKey)
	}

	return result, nil
}

func get(w http.ResponseWriter, l *hkp.Lookup) {
	keys, err := keys(l)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(keys) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	err = openpgp.WriteArmoredPackets(w, keys)
	if err != nil {
		log.Printf("get %q: error writing armored keys: %v", l.Search, err)
	}
}
