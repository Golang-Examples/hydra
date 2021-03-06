package oauth2

import (
	"net/http"
	"net/url"

	"github.com/go-errors/errors"
	"github.com/julienschmidt/httprouter"
	"github.com/ory-am/fosite"
	csh "github.com/ory-am/fosite/handler/core/strategy"
	"github.com/ory-am/fosite/handler/oidc/strategy"
	"github.com/ory-am/fosite/token/jwt"
	"github.com/ory-am/hydra/pkg"
)

const (
	OpenIDConnectKeyName = "hydra.openid.connect"
)

type Handler struct {
	OAuth2  fosite.OAuth2Provider
	Consent ConsentStrategy

	ConsentURL url.URL
}

func (h *Handler) SetRoutes(r *httprouter.Router) {
	r.POST("/oauth2/token", h.TokenHandler)
	r.GET("/oauth2/auth", h.AuthHandler)
	r.POST("/oauth2/auth", h.AuthHandler)
}

func (o *Handler) TokenHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var session = Session{
		DefaultSession: &strategy.DefaultSession{
			Claims:      new(jwt.IDTokenClaims),
			Headers:     new(jwt.Headers),
			HMACSession: new(csh.HMACSession),
		},
	}
	var ctx = fosite.NewContext()

	accessRequest, err := o.OAuth2.NewAccessRequest(ctx, r, &session)
	if err != nil {
		pkg.LogError(err)
		o.OAuth2.WriteAccessError(w, accessRequest, err)
		return
	}

	if accessRequest.GetGrantTypes().Exact("client_credentials") {
		session.Subject = accessRequest.GetClient().GetID()
	}

	accessResponse, err := o.OAuth2.NewAccessResponse(ctx, r, accessRequest)
	if err != nil {
		pkg.LogError(err)
		o.OAuth2.WriteAccessError(w, accessRequest, err)
		return
	}

	o.OAuth2.WriteAccessResponse(w, accessRequest, accessResponse)
}

func (o *Handler) AuthHandler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var ctx = fosite.NewContext()

	authorizeRequest, err := o.OAuth2.NewAuthorizeRequest(ctx, r)
	if err != nil {
		pkg.LogError(err)
		o.writeAuthorizeError(w, authorizeRequest, err)
		return
	}

	// A session_token will be available if the user was authenticated an gave consent
	consentToken := authorizeRequest.GetRequestForm().Get("consent")
	if consentToken == "" {
		// otherwise redirect to log in endpoint
		if err := o.redirectToConsent(w, r, authorizeRequest); err != nil {
			pkg.LogError(err)
			o.writeAuthorizeError(w, authorizeRequest, err)
			return
		}
		return
	}

	// decode consent_token claims
	// verify anti-CSRF (inject state) and anti-replay token (expiry time, good value would be 10 seconds)
	session, err := o.Consent.ValidateResponse(authorizeRequest, consentToken)
	if err != nil {
		pkg.LogError(err)
		o.writeAuthorizeError(w, authorizeRequest, errors.New(fosite.ErrAccessDenied))
		return
	}

	// done
	response, err := o.OAuth2.NewAuthorizeResponse(ctx, r, authorizeRequest, session)
	if err != nil {
		pkg.LogError(err)
		o.writeAuthorizeError(w, authorizeRequest, err)
		return
	}

	o.OAuth2.WriteAuthorizeResponse(w, authorizeRequest, response)
}

func (o *Handler) redirectToConsent(w http.ResponseWriter, r *http.Request, authorizeRequest fosite.AuthorizeRequester) error {
	schema := "https"
	if r.TLS == nil {
		schema = "http"
	}
	challenge, err := o.Consent.IssueChallenge(authorizeRequest, schema+"://"+r.Host+r.URL.String())
	if err != nil {
		return err
	}

	p := o.ConsentURL
	q := p.Query()
	q.Set("challenge", challenge)
	p.RawQuery = q.Encode()
	http.Redirect(w, r, p.String(), http.StatusFound)
	return nil
}

func (o *Handler) writeAuthorizeError(w http.ResponseWriter, ar fosite.AuthorizeRequester, err error) {
	if !ar.IsRedirectURIValid() {
		var rfcerr = fosite.ErrorToRFC6749Error(err)

		redirectURI := o.ConsentURL
		query := redirectURI.Query()
		query.Add("error", rfcerr.Name)
		query.Add("error_description", rfcerr.Description)
		redirectURI.RawQuery = query.Encode()

		w.Header().Add("Location", redirectURI.String())
		w.WriteHeader(http.StatusFound)
		return
	}

	o.OAuth2.WriteAuthorizeError(w, ar, err)
}
