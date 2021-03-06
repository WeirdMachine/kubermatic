// Code generated by go-swagger; DO NOT EDIT.

package versions

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
)

// NewGetMasterVersionsParams creates a new GetMasterVersionsParams object
// with the default values initialized.
func NewGetMasterVersionsParams() *GetMasterVersionsParams {

	return &GetMasterVersionsParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetMasterVersionsParamsWithTimeout creates a new GetMasterVersionsParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetMasterVersionsParamsWithTimeout(timeout time.Duration) *GetMasterVersionsParams {

	return &GetMasterVersionsParams{

		timeout: timeout,
	}
}

// NewGetMasterVersionsParamsWithContext creates a new GetMasterVersionsParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetMasterVersionsParamsWithContext(ctx context.Context) *GetMasterVersionsParams {

	return &GetMasterVersionsParams{

		Context: ctx,
	}
}

// NewGetMasterVersionsParamsWithHTTPClient creates a new GetMasterVersionsParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetMasterVersionsParamsWithHTTPClient(client *http.Client) *GetMasterVersionsParams {

	return &GetMasterVersionsParams{
		HTTPClient: client,
	}
}

/*GetMasterVersionsParams contains all the parameters to send to the API endpoint
for the get master versions operation typically these are written to a http.Request
*/
type GetMasterVersionsParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get master versions params
func (o *GetMasterVersionsParams) WithTimeout(timeout time.Duration) *GetMasterVersionsParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get master versions params
func (o *GetMasterVersionsParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get master versions params
func (o *GetMasterVersionsParams) WithContext(ctx context.Context) *GetMasterVersionsParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get master versions params
func (o *GetMasterVersionsParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get master versions params
func (o *GetMasterVersionsParams) WithHTTPClient(client *http.Client) *GetMasterVersionsParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get master versions params
func (o *GetMasterVersionsParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *GetMasterVersionsParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
