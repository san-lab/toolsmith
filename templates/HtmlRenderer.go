package templates

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"strings"
)

type Renderer struct {
	templates *template.Template
}

func NewRenderer() *Renderer {
	r := &Renderer{}
	r.LoadTemplates()
	return r
}

const Raw string = "raw"
const Home string = "home"
const Network = "network"
const Peers = "peers"
const ListMap = "listMap"
const TxpoolStatus = "txpoolStatus"
const BlockNumber =  "blockNumber"


//Taken out of the constructor with the idae of forced template reloading
//TODO: error handling
func (r *Renderer) LoadTemplates() {
	var allFiles []string
	files, err := ioutil.ReadDir("./templates")
	if err != nil {
		fmt.Println(err)
	}
	for _, file := range files {
		filename := file.Name()
		if strings.HasSuffix(filename, ".htemplate") {
			allFiles = append(allFiles, "./templates/"+filename)
		}
	}
	r.templates, err = template.ParseFiles(allFiles...) //parses all .tmpl files in the 'templates' folder
	if err != nil {
		fmt.Println(err)
	}
}

func (r *Renderer) RenderResponse(w io.Writer, data RenderData) error {
	return r.templates.ExecuteTemplate(w, data.TemplateName, data)

}


//This is a try to bring some uniformity to passing data to the templates
//The "RenderData" container is a wrapper for the header/body/footer containers
type RenderData struct {
	Error string
	TemplateName string
	HeaderData interface{}
	BodyData interface{}
	FooterData interface{}
	Client interface{}
}


