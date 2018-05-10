
// +build !awsmail


package mailer

import "log"

func SendEmail(to []*string, subject string, htmlBody string, plainTextBody string) {
	rec := ""
	for _, v := range to {
		rec = rec + *v + ";"
	}
	log.Println("Dummy sendmail to: ", rec)
	log.Println("subject: ", subject)
	log.Println( "html: ", htmlBody)
	log.Println("plain text: ")
	log.Println("----------")
}
