Mockingbird - Generic HTTP API mocking framework
================================================

Has a 'database' of request / response pairs which you are free to call 'test
cases'. Real requests against the API mock are matched against those cases:

All volatile HTTP headers are stripped (like User-Agent, Date,
If-Modified-Since, ...). Sanitized requests are then hashed and compared
against our (equally sanitized on hashed) list of request test cases. If a
match is found the corresponding respond is returned.

In other words, this API endpoint mocking framework is totally agnostic of
the payload. You can deliver SOAP, REST or binary blob test cases as long as
your API uses HTTP request / response objects.
