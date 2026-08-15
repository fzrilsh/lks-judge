package automark

// ExampleConfigJSON is a complete, working suite that exercises every feature
// the visual builder can express: base scheme/port/path, global auth with a
// {{uniqid}} login body and a tokenPath, both note lists ordered high to low,
// two groups, and assertions covering requires_auth, invalidates_token, a
// request body, deduction present (record but keep score) and absent (any fail
// zeroes it), a status/message scalar check, a nested object shape, and a
// list-of shape. The Load-example button drops this into the JSON textarea so
// an empty config is approachable. A test keeps it parsing and validating.
const ExampleConfigJSON = `{
  "base": {
    "scheme": "http",
    "port": 8080,
    "path": "/api"
  },
  "auth": {
    "login": {
      "method": "POST",
      "endpoint": "/login",
      "body": {
        "email": "judge{{uniqid}}@example.com",
        "password": "secret"
      }
    },
    "tokenPath": "data.token"
  },
  "grading": {
    "groupNotes": [
      { "min": 80, "text": "Excellent" },
      { "min": 50, "text": "Adequate" },
      { "min": 0, "text": "Needs work" }
    ],
    "totalNotes": [
      { "min": 75, "text": "Pass" },
      { "min": 0, "text": "Fail" }
    ]
  },
  "groups": [
    {
      "group_id": "auth",
      "group_name": "Authentication",
      "assertions": [
        {
          "title": "Register a new user",
          "method": "POST",
          "endpoint": "/register",
          "request": {
            "body": {
              "email": "user{{uniqid}}@example.com",
              "password": "secret"
            }
          },
          "expected": {
            "status_code": 201,
            "body": {
              "status": "success",
              "message": "Registered",
              "data": ["id", "token"]
            }
          },
          "score": 10,
          "deduction": 2.5
        },
        {
          "title": "Reject a bad login",
          "method": "POST",
          "endpoint": "/login",
          "request": {
            "body": {
              "email": "nobody@example.com",
              "password": "wrong"
            }
          },
          "expected": {
            "status_code": 401,
            "body": {
              "status": "error"
            }
          },
          "score": 5
        }
      ]
    },
    {
      "group_id": "profile",
      "group_name": "Profile",
      "assertions": [
        {
          "title": "Fetch the current profile",
          "method": "GET",
          "endpoint": "/me",
          "requires_auth": true,
          "expected": {
            "status_code": 200,
            "body": {
              "data": {
                "0": "id",
                "profile": ["name", "email"],
                "roles": { "*": ["id", "label"] }
              }
            }
          },
          "score": 10,
          "deduction": 0
        },
        {
          "title": "Log out",
          "method": "POST",
          "endpoint": "/logout",
          "requires_auth": true,
          "invalidates_token": true,
          "expected": {
            "status_code": 200,
            "body": {
              "message": "Logged out"
            }
          },
          "score": 5
        }
      ]
    }
  ]
}`
