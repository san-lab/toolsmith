package mailer

import (
	"sync"
	"html/template"

	"bytes"
	"log"
)

type Mailer struct {
	templateLoaded        bool
	alertTemplateFilename string
	AlertTmpl             *template.Template
	overTemplateFilename string
	OverTmpl 			*template.Template
}

var m  *Mailer
var once sync.Once
const templatefilename = "./templates/mailalert.mtemplate"
const overtemplatefilename ="./templates/mailalertover.mtemplate"

func GetMailer() *Mailer {
	once.Do(initMailer)
	return m
}

func initMailer() {
	m = &Mailer{alertTemplateFilename:templatefilename, overTemplateFilename:overtemplatefilename}
	m.LoadTemplate()
}

func (m *Mailer) LoadTemplate() bool {
	var err, err1 error
	m.AlertTmpl, err = template.ParseFiles(m.alertTemplateFilename)
	m.OverTmpl, err1 = template.ParseFiles(m.overTemplateFilename)
	m.templateLoaded=err==nil&&err1==nil
	return m.templateLoaded
}


//Execute the email template against a data struct
//As of this writing expected data is:
// {.IssueID}
// {.Severity}
// {.WachdogAddress}
// {.UnreachableNodes}
// {.StuckNodes}
//
func (m *Mailer) RenderAlert(data interface{}) string {
	if !m.templateLoaded {
		return "Email template not loaded"
	}
	buf := new(bytes.Buffer)
	if err := m.AlertTmpl.Execute(buf, data); err != nil {
		log.Println(err)
		return "Error executing email template"
	}
	return buf.String()
}

//Alert is over
func (m *Mailer) RenderOver(issueid string) string {
	if !m.templateLoaded {
		return "Email template not loaded"
	}
	buf := new(bytes.Buffer)
	if err := m.OverTmpl.Execute(buf, struct{IssueID string}{ issueid}); err != nil {
		log.Println(err)
		return "Error executing email template"
	}
	return buf.String()
}

