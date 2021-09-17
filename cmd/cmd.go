package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"runtime"
	"text/template"

	"gopkg.in/yaml.v2"
)

var (
	infoLogger  *log.Logger
	fatalLogger *log.Logger
	errorLogger *log.Logger
)

const (
	helpMsgConfigFile string = "config file path"
)

//Config unites all following configs into a single type
type MailConfig struct {
	Sender       SenderConfig         `yaml:"Sender"`
	Recipients   map[string]Recipient `yaml:"Recipients"`
	Header       Header               `yaml:"Header"`
	TemplateText string               `yaml:"TemplateText"`

	//template can contain whatever is in struct EmailSendRequest
	template *template.Template
}

//SenderConfig describes from who and which host we should
//send the emails
type SenderConfig struct {
	Host     string `yaml:"ServerHost"`
	Port     int    `yaml:"ServerPort"`
	Address  string `yaml:"SenderAddress"`
	Name     string `yaml:"SenderName"`
	Password string `yaml:"SenderPassword"`
}

//Header is the email header.
type Header struct {
	From string `yaml:"From"`
	//To            string `yaml:"To"`
	Subject       string `yaml:"Subject"`
	MIME          string `yaml:"MIME"`
	Miscellaneous string `yaml:"Miscellaneous"`
}

//Recipient is a person who receives an email. Parameters here
//are used in email template. the email is sent to `Address`
type Recipient struct {
	Name          string      `yaml:"Name"`
	Title         string      `yaml:"Title"`
	Address       string      `yaml:"Address"`
	Miscellaneous interface{} `yaml:"Miscellaneous"`
}

type EmailSendRequest struct {
	IPAddress    string
	FirstName    string
	LastName     string
	CompanyName  string
	EmailAddress string
	Description  string
	Result       chan<- EmailSendOutcome
}

type EmailSendOutcome struct {
	Error error
}

type ServerConfig struct {
	Address string `yaml:"Address"`
	BaseURL string `yaml:"BaseURL"`

	EmailConfig MailConfig `yaml:"EmailConfig"`
}

type server struct {
	config      ServerConfig
	emailSender chan<- EmailSendRequest
}

func (h *Header) ToString(to string) string {
	return fmt.Sprintf(
		"From: %s\nTo: %s\nSubject: %s\n%s\n%s\n",
		h.From,
		to,
		h.Subject,
		h.MIME,
		h.Miscellaneous,
	)
}

func checkFatalError(err error, stage string) {
	if err != nil {
		fatalLogger.Fatalf("@%s: %v\n", stage, err)
	}
}

func (c *ServerConfig) getConfig(filename string) error {
	c.EmailConfig.Recipients = make(map[string]Recipient)

	yamlFile, err := ioutil.ReadFile(filename)
	checkFatalError(err, "READING CONFIG FILE")

	err = yaml.Unmarshal(yamlFile, c)
	checkFatalError(err, "PARSING CONFIG FILE")

	c.EmailConfig.template, err = template.New("Body").Parse(c.EmailConfig.TemplateText)
	checkFatalError(err, "PARSING EMAIL TEMPLATE")

	return nil
}

func (m *MailConfig) EmailerInstance(ch <-chan EmailSendRequest) {
	auth := smtp.PlainAuth(
		"",
		m.Sender.Address,
		m.Sender.Password,
		m.Sender.Host,
	)
	address := fmt.Sprintf("%s:%d", m.Sender.Host, m.Sender.Port)
	var err error
	for cmd := range ch {
		buf := new(bytes.Buffer)
		err = m.template.Execute(buf, cmd)
		if err != nil {
			cmd.Result <- EmailSendOutcome{err}
			continue
		}
		for _, r := range m.Recipients {
			err = smtp.SendMail(
				address,
				auth,
				m.Sender.Address,
				[]string{r.Address},
				[]byte(m.Header.ToString(r.Address)+buf.String()),
			)
			/*
				infoLogger.Printf("Wanted to send message %s with header %s to address %s, recipient %s",
					buf.String(), m.Header.ToString(r.Address), address, r.Name)
			*/
			cmd.Result <- EmailSendOutcome{err}
		}
	}
}

func (s *server) clientHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid Form", http.StatusBadRequest)
			return
		}
		var data EmailSendRequest
		data.IPAddress = r.RemoteAddr
		data.FirstName = r.FormValue("firstName")
		data.LastName = r.FormValue("lastName")
		data.CompanyName = r.FormValue("company")
		data.EmailAddress = r.FormValue("email")
		data.Description = r.FormValue("description")
		result := make(chan EmailSendOutcome)
		data.Result = result
		s.emailSender <- data
		outcome := <-result
		if outcome.Error != nil {
			errorLogger.Printf(
				"Error handling client (IP: %s, Name: %s, Company: %s, Email: %s): %v",
				data.IPAddress,
				data.FirstName+" "+data.LastName,
				data.CompanyName,
				data.EmailAddress,
				outcome.Error,
			)
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Success!")
	default:
		http.Error(w, "Invalid request", http.StatusNotImplemented)
	}
}

func Execute() {
	var cfg ServerConfig
	var configFile string
	var defaultConfigFile string = "config.yaml"

	if runtime.GOOS == "linux" {
		defaultConfigFile = "/etc/docs-email-sender/config.yaml"
	}

	infoLogger = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Llongfile)
	fatalLogger = log.New(os.Stderr, "FATAL: ", log.Ldate|log.Ltime|log.Llongfile)
	errorLogger = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

	flag.StringVar(&configFile, "c", defaultConfigFile, helpMsgConfigFile+" (shortened)")
	flag.StringVar(&configFile, "configFile", defaultConfigFile, helpMsgConfigFile)
	flag.Parse()

	err := cfg.getConfig(configFile)
	checkFatalError(err, "READING/PARSING CONFIG FILE")
	infoLogger.Println("Successfuly Read Config File")

	emailChan := make(chan EmailSendRequest)
	go cfg.EmailConfig.EmailerInstance(emailChan)

	s := &server{}
	s.config = cfg
	s.emailSender = emailChan

	http.HandleFunc(s.config.BaseURL, s.clientHandler) //TODO: Complete clientHandler
	infoLogger.Println("Successfuly Initialized WebServer")
	infoLogger.Printf("Serving at %s\n", s.config.Address)

	os.Stdout.Sync()

	fatalLogger.Fatal(http.ListenAndServe(s.config.Address, nil))

}
