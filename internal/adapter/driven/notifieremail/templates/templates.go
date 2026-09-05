// Package templates provides functionality to render email templates for account verification.
package templates

import (
	"bytes"
	"errors"
	"html/template"
)

var (
	ErrInvalidAccountVerificationConf = errors.New("invalid account verification conf")
	ErrInvalidVerificationWebEndpoint = errors.New("invalid verification API endpoint")
	ErrInvalidVerificationToken       = errors.New("invalid verification token")
	ErrInvalidVerificationTTL         = errors.New("invalid verification TTL")
	ErrInvalidUserName                = errors.New("invalid user name")
)

const accountVerificationHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Account Verification</title>
</head>

<body>
    <h1>Account Verification</h1>
    <p>Dear User {{.UserName}},</p>
    <p>Thank you for signing up!</p>
    <p>To complete your registration, please verify your account.</p>
    <p>We are excited to have you on board!</p>
    <p>To verify your account, please click the link below:</p>
    <a href="{{.VerificationWebEndpoint}}?token={{.VerificationToken}}">Verify Account</a>
    <br/>
    <h3>Token will expire in {{.VerificationTTL}}</h3>
</body>
</html>
`

const accountVerificationText = `
Account Verification

Dear User {{.UserName}},
Thank you for signing up!
To complete your registration, please verify your account.
We are excited to have you on board!
To verify your account, please click the link below:

{{.VerificationWebEndpoint}}?token={{.VerificationToken}}

Token will expire in {{.VerificationTTL}}
`

// EmailAccountVerificationConf holds the configuration for the email account verification template.
type EmailAccountVerificationConf struct {
	VerificationWebEndpoint string
	VerificationToken       string
	VerificationTTL         string
	UserName                string
	HTML                    bool
}

// EmailAccountVerification represents the email account verification template.
type EmailAccountVerification struct {
	verificationAPIEndpoint string
	verificationToken       string
	verificationTTL         string
	userName                string
	html                    bool
}

// NewEmailAccountVerification creates a new EmailAccountVerification instance.
func NewEmailAccountVerification(conf *EmailAccountVerificationConf) (*EmailAccountVerification, error) {
	if conf == nil {
		return nil, ErrInvalidAccountVerificationConf
	}

	if conf.VerificationWebEndpoint == "" {
		return nil, ErrInvalidVerificationWebEndpoint
	}

	if conf.VerificationToken == "" {
		return nil, ErrInvalidVerificationToken
	}

	if conf.VerificationTTL == "" {
		return nil, ErrInvalidVerificationTTL
	}

	if conf.UserName == "" {
		return nil, ErrInvalidUserName
	}

	return &EmailAccountVerification{
		verificationAPIEndpoint: conf.VerificationWebEndpoint,
		verificationToken:       conf.VerificationToken,
		verificationTTL:         conf.VerificationTTL,
		userName:                conf.UserName,
		html:                    conf.HTML,
	}, nil
}

func (e *EmailAccountVerification) Render() string {
	var tmplType string

	if e.html {
		tmplType = accountVerificationHTML
	} else {
		tmplType = accountVerificationText
	}

	data := struct {
		VerificationWebEndpoint string
		VerificationToken       string
		VerificationTTL         string
		UserName                string
	}{
		VerificationWebEndpoint: e.verificationAPIEndpoint,
		VerificationToken:       e.verificationToken,
		VerificationTTL:         e.verificationTTL,
		UserName:                e.userName,
	}

	tmpl, err := template.New("accountVerification").Parse(tmplType)
	if err != nil {
		panic(err)
	}

	var tpl bytes.Buffer
	err = tmpl.Execute(&tpl, data)
	if err != nil {
		panic(err)
	}

	return tpl.String()
}

const resetPasswordHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Reset Password</title>
</head>

<body>
    <h1>Reset Password</h1>
    <p>Dear User {{.UserName}},</p>
    <p>We received a request to reset your password.</p>
    <p>If you did not make this request, please ignore this email.</p>
    <p>To reset your password, please click the link below:</p>
    <a href="{{.ResetPasswordAPIEndpoint}}?token={{.ResetPasswordToken}}">Reset Password</a>
    <br/>
    <h3>Token will expire in {{.ResetPasswordTTL}}</h3>
</body>
</html>
`

const resetPasswordText = `
Reset Password

Dear User {{.UserName}},
We received a request to reset your password.
If you did not make this request, please ignore this email.
To reset your password, please click the link below:

{{.ResetPasswordAPIEndpoint}}?token={{.ResetPasswordToken}}

Token will expire in {{.ResetPasswordTTL}}
`

type EmailResetPasswordConf struct {
	ResetPasswordAPIEndpoint string
	ResetPasswordToken       string
	ResetPasswordTTL         string
	UserName                 string
	HTML                     bool
}

type EmailResetPassword struct {
	resetPasswordAPIEndpoint string
	resetPasswordToken       string
	resetPasswordTTL         string
	userName                 string
	html                     bool
}

func NewEmailResetPassword(conf *EmailResetPasswordConf) (*EmailResetPassword, error) {
	if conf == nil {
		return nil, errors.New("invalid reset password conf")
	}

	if conf.ResetPasswordAPIEndpoint == "" {
		return nil, errors.New("invalid reset password API endpoint")
	}

	if conf.ResetPasswordToken == "" {
		return nil, errors.New("invalid reset password token")
	}

	if conf.ResetPasswordTTL == "" {
		return nil, errors.New("invalid reset password TTL")
	}

	if conf.UserName == "" {
		return nil, errors.New("invalid user name")
	}

	return &EmailResetPassword{
		resetPasswordAPIEndpoint: conf.ResetPasswordAPIEndpoint,
		resetPasswordToken:       conf.ResetPasswordToken,
		resetPasswordTTL:         conf.ResetPasswordTTL,
		userName:                 conf.UserName,
		html:                     conf.HTML,
	}, nil
}

func (r *EmailResetPassword) Render() string {
	var tmplType string

	if r.html {
		tmplType = resetPasswordHTML
	} else {
		tmplType = resetPasswordText
	}

	data := struct {
		ResetPasswordAPIEndpoint string
		ResetPasswordToken       string
		ResetPasswordTTL         string
		UserName                 string
	}{
		ResetPasswordAPIEndpoint: r.resetPasswordAPIEndpoint,
		ResetPasswordToken:       r.resetPasswordToken,
		ResetPasswordTTL:         r.resetPasswordTTL,
		UserName:                 r.userName,
	}

	tmpl, err := template.New("resetPassword").Parse(tmplType)
	if err != nil {
		panic(err)
	}

	var tpl bytes.Buffer
	err = tmpl.Execute(&tpl, data)
	if err != nil {
		panic(err)
	}

	return tpl.String()
}

// The email an existing account gets when someone tries to register with its
// address.
//
// Registration answers the same way whether or not the address is taken, so
// this is the ONLY way the real owner learns it happened -- and the only way a
// person who has simply forgotten they have an account finds out, instead of
// being told "registered" and then never receiving anything.
//
// It carries no token and no link that acts on the account. Anyone can cause
// this mail to be sent to any address, so it must not be an instruction the
// recipient can be walked through; it points at the sign-in page and nothing
// else.
const accountExistsHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>You already have an account</title>
</head>

<body>
    <h1>You already have an account</h1>
    <p>Dear User {{.UserName}},</p>
    <p>Someone just tried to create an account with this email address, and one already exists.</p>
    <p>If that was you, there is nothing to do -- sign in as usual, and use "forgot password" if you cannot remember your password.</p>
    <p>If it was not you, you can ignore this email. Nothing about your account has changed, and no one was told whether this address is registered.</p>
    <a href="{{.SignInURL}}">Go to sign in</a>
</body>
</html>
`

const accountExistsText = `
You already have an account

Dear User {{.UserName}},
Someone just tried to create an account with this email address, and one already exists.
If that was you, there is nothing to do -- sign in as usual, and use "forgot password" if you cannot remember your password.
If it was not you, you can ignore this email. Nothing about your account has changed, and no one was told whether this address is registered.

{{.SignInURL}}
`

type EmailAccountExistsConf struct {
	SignInURL string
	UserName  string
	HTML      bool
}

type EmailAccountExists struct {
	signInURL string
	userName  string
	html      bool
}

func NewEmailAccountExists(conf *EmailAccountExistsConf) (*EmailAccountExists, error) {
	if conf == nil {
		return nil, errors.New("invalid account exists conf")
	}

	if conf.SignInURL == "" {
		return nil, errors.New("invalid sign in URL")
	}

	if conf.UserName == "" {
		return nil, errors.New("invalid user name")
	}

	return &EmailAccountExists{
		signInURL: conf.SignInURL,
		userName:  conf.UserName,
		html:      conf.HTML,
	}, nil
}

func (r *EmailAccountExists) Render() string {
	tmplType := accountExistsText
	if r.html {
		tmplType = accountExistsHTML
	}

	data := struct {
		SignInURL string
		UserName  string
	}{
		SignInURL: r.signInURL,
		UserName:  r.userName,
	}

	tmpl, err := template.New("accountExists").Parse(tmplType)
	if err != nil {
		panic(err)
	}

	var tpl bytes.Buffer
	if err := tmpl.Execute(&tpl, data); err != nil {
		panic(err)
	}

	return tpl.String()
}
