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
	var err error
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
	return r
}

func (r *Renderer) RenderResponse(w io.Writer, name string, data interface{}) {
	r.templates.ExecuteTemplate(w, name, data)
	return
}
