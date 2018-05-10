
// +build !awsmail


package mailer

import "log"

func SendEmail(to []*string, subject string, htmlBody string, plainTextBody string) {
	var adr string
	if len(to) > 0 {adr = *to[0]} else {adr="noone"}
log.Printf("email to %s about %s, %s", adr, subject, plainTextBody)
}
