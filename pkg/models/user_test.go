package models

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"reflect"
	"testing"

	"github.com/francoispqt/gojay"
	"github.com/wvh/uuid"
)

var baseUser = &User{
	Uid:          uuid.MustFromString("053bffbcc41edad4853bea91fc42ea18"),
	Identity:     "jack@openid",
	Service:      "openid",
	Name:         "Jack",
	Email:        "jack@example.com",
	Organisation: "Jack, Inc.",
	Projects:     []string{"Project X", "Project 666"},
}

func withoutProjects(user *User) *User {
	newUser := *user
	newUser.Projects = []string{}
	//newUser.Projects = nil
	return &newUser
}

func withoutUid(user *User) *User {
	newUser := *user
	newUser.Uid = [16]byte{}
	return &newUser
}

// isNilAndEmpty makes it possible to compare a nil slice with an empty slice.
func isNilAndEmpty(a, b []string) bool {
	return (a == nil && len(b) == 0 || len(a) == 0 && b == nil)
}

var tests = []struct {
	name string
	user *User
	json string
}{
	{
		name: "baseUser",
		user: baseUser,
		json: `{"uid":"053bffbcc41edad4853bea91fc42ea18","identity":"jack@openid","service":"openid","name":"Jack","email":"jack@example.com","organisation":"Jack, Inc.","projects":["Project X","Project 666"]}`,
	},
	{
		name: "withoutUid",
		user: withoutUid(baseUser),
		json: `{"uid":"00000000000000000000000000000000","identity":"jack@openid","service":"openid","name":"Jack","email":"jack@example.com","organisation":"Jack, Inc.","projects":["Project X","Project 666"]}`,
	},
	{
		name: "withoutProjects",
		user: withoutProjects(baseUser),
		json: `{"uid":"053bffbcc41edad4853bea91fc42ea18","identity":"jack@openid","service":"openid","name":"Jack","email":"jack@example.com","organisation":"Jack, Inc."}`,
	},
}

func TestUserToJson(t *testing.T) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			json, err := gojay.MarshalJSONObject(test.user)
			if err != nil {
				t.Fatal(err)
			}

			if string(json) != test.json {
				t.Errorf("error serialising user object:\n\texpected %q,\n\tgot      %q", test.json, json)
			}
		})
	}
}

func TestJsonToUser(t *testing.T) {
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			user := new(User)
			err := gojay.UnmarshalJSONObject([]byte(test.json), user)
			if err != nil {
				t.Fatal(err)
			}

			if user.Uid.String() != test.user.Uid.String() {
				t.Errorf("error deserialising user.Uid: expected %q, got %q", test.user.Uid, user.Uid)
			}
			if user.Email != test.user.Email {
				t.Errorf("error deserialising user.Email: expected %q, got %q", test.user.Email, user.Email)
			}
			// for the purpose of this test, we consider a nil slice equal to an empty one
			if !reflect.DeepEqual(user.Projects, test.user.Projects) && !isNilAndEmpty(user.Projects, test.user.Projects) {
				t.Errorf("error deserialising user.Projects: expected %#v, got %#v", test.user.Projects, user.Projects)
			}
		})
	}
}

func TestNullJsonUser(t *testing.T) {
	json := `{"uid": null, "identity":"jack@openid","name":"Jack","email":"jack@example.com","organisation":"Jack, Inc."}`
	user := new(User)

	// this gets decoded as empty value, so returns invalid UUID
	err := gojay.UnmarshalJSONObject([]byte(json), user)
	if err != uuid.ErrInvalidUUID {
		t.Fatal(err)
	}

	t.Logf("%+v\n", user.Uid)
}

func TestMissingJsonUser(t *testing.T) {
	json := `{"identity":"jack@openid","name":"Jack","email":"jack@example.com","organisation":"Jack, Inc."}`
	user := new(User)

	// this gets skipped because the field is missing
	err := gojay.UnmarshalJSONObject([]byte(json), user)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%+v\n", user.Uid)
}

func TestAddAccessGranter(t *testing.T) {
	str := baseUser.AddAccessGranter("test=1234&test2=1337")

	values, err := url.ParseQuery(str)
	if err != nil {
		t.Fatal(err)
	}

	// Existing query parameters should not change
	if values.Get("test") != "1234" {
		t.Errorf("value of query parameter test changed")
	}
	if values.Get("test2") != "1337" {
		t.Errorf("value of query parameter test2 changed")
	}

	// The access_granter parameter should be base64 encoded json
	encoded := values.Get("access_granter")
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	accessGranter := new(AccessGranter)
	err = json.Unmarshal([]byte(decoded), accessGranter)
	if err != nil {
		t.Fatal(err)
	}

	// The values contained in access_granter should match the User
	if accessGranter.UserID != baseUser.Identity {
		t.Errorf("accessGranter.UserID != baseUser.Identity")
	}
	if accessGranter.Email != baseUser.Email {
		t.Errorf("accessGranter.Email != baseUser.Email")
	}
	if accessGranter.Name != baseUser.Name {
		t.Errorf("accessGranter.Name != baseUser.Name")
	}
}
