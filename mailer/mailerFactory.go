

package mailer


type MailSender interface {
	SendMail(to []*string, subject string, html string, plain string)
}

