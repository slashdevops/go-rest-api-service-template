package domain

import (
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"

	"uuid"
)

func TestEncodeToken(t *testing.T) {
	type args struct {
		id        uuid.UUID
		serial    int64
		direction TokenDirection
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "success",
			args: args{
				id:        uuid.Max(),
				serial:    0,
				direction: TokenDirectionNext,
			},
			want: base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;0;2")),
		},
		{
			name: "success with direction prev",
			args: args{
				id:        uuid.Max(),
				serial:    1,
				direction: TokenDirectionPrev,
			},
			want: base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;1;1")),
		},
		{
			name: "success with direction next",
			args: args{
				id:        uuid.Max(),
				serial:    2,
				direction: TokenDirectionNext,
			},
			want: base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;2;2")),
		},
		{
			name: "success with direction invalid",
			args: args{
				id:        uuid.Max(),
				serial:    3,
				direction: TokenDirectionInvalid,
			},
			want: base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;3;3")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EncodeToken(tt.args.id, tt.args.serial, tt.args.direction); got != tt.want {
				t.Errorf("EncodeToken() = %v, want %v", got, tt.want)
			}

			// Decode the token to verify the encoding
			id, date, direction, err := DecodeToken(tt.want, tt.args.direction)

			if tt.args.direction == TokenDirectionInvalid {
				if err == nil {
					t.Errorf("DecodeToken() expected an error for TokenDirectionInvalid, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("DecodeToken() error = %v", err)
				}

				if date != tt.args.serial {
					t.Errorf("DecodeToken() date = %v, want %v", date, tt.args.serial)
				}

				if id != tt.args.id {
					t.Errorf("DecodeToken() id = %v, want %v", id, tt.args.id)
				}

				if direction != tt.args.direction {
					t.Errorf("DecodeToken() direction = %v, want %v", direction, tt.args.direction)
				}
			}
		})
	}
}

func TestDecodeToken(t *testing.T) {
	type args struct {
		s   string
		dir TokenDirection
	}
	tests := []struct {
		name          string
		args          args
		wantSerial    int64
		wantID        uuid.UUID
		wantDirection TokenDirection
		wantErr       bool
	}{
		{
			name: "success with next direction",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;0;2")),
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Max(),
			wantDirection: TokenDirectionNext,
			wantErr:       false,
		},
		{
			name: "success with prev direction",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;10;1")),
				dir: TokenDirectionPrev,
			},
			wantSerial:    10,
			wantID:        uuid.Max(),
			wantDirection: TokenDirectionPrev,
			wantErr:       false,
		},
		{
			name: "invalid token",
			args: args{
				s:   "invalid",
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
		{
			name: "invalid format - missing separator",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff")),
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
		{
			name: "invalid format - not enough separators",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;0")),
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
		{
			name: "invalid format - too many separators",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;0;2;extra")),
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
		{
			name: "invalid uuid",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("not-a-uuid;0;2")),
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
		{
			name: "invalid serial",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;not-a-number;2")),
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
		{
			name: "invalid direction",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;0;not-a-number")),
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
		{
			name: "direction out of range",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;0;5")),
				dir: TokenDirectionNext,
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
		{
			name: "error when decoding token with invalid direction",
			args: args{
				s:   base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;3;3")),
				dir: TokenDirectionNext, // The dir argument here is for the expected direction if successful,
				// but we expect an error due to the embedded invalid direction.
			},
			wantSerial:    0,
			wantID:        uuid.Nil(),
			wantDirection: TokenDirectionInvalid,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotSerial, gotDirection, gotErr := DecodeToken(tt.args.s, tt.args.dir)
			if (gotErr != nil) != tt.wantErr {
				t.Errorf("DecodeToken() error = %v, wantErr %v", gotErr, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if !reflect.DeepEqual(gotSerial, tt.wantSerial) {
					t.Errorf("DecodeToken() gotSerial = %v, want %v", gotSerial, tt.wantSerial)
				}
				if !reflect.DeepEqual(gotID, tt.wantID) {
					t.Errorf("DecodeToken() gotID = %v, want %v", gotID, tt.wantID)
				}
				if gotDirection != tt.wantDirection {
					t.Errorf("DecodeToken() gotDirection = %v, want %v", gotDirection, tt.wantDirection)
				}

				// Encode the token to verify the decoding
				token := EncodeToken(gotID, gotSerial, gotDirection)
				if token != tt.args.s {
					t.Errorf("EncodeToken() = %v, want %v", token, tt.args.s)
				}
			}
		})
	}
}

func TestPaginator_GenerateToken(t *testing.T) {
	type fields struct {
		NextToken string
		NextPage  string
		PrevToken string
		PrevPage  string
		Size      int
		Limit     int
	}
	type args struct {
		id        uuid.UUID
		serial    int64
		direction TokenDirection
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{
			name: "success with uuid.Max() and next direction",
			fields: fields{
				Limit: 10,
			},
			args: args{
				id:        uuid.Max(),
				serial:    0,
				direction: TokenDirectionNext,
			},
			want: base64.StdEncoding.EncodeToString([]byte("ffffffff-ffff-ffff-ffff-ffffffffffff;0;2")),
		},
		{
			name: "success with random uuid and prev direction",
			fields: fields{
				Limit: 20,
			},
			args: args{
				id:        uuid.MustParse("f47ac10b-58cc-0372-8567-0e02b2c3d479"),
				serial:    1234567890,
				direction: TokenDirectionPrev,
			},
			want: base64.StdEncoding.EncodeToString([]byte("f47ac10b-58cc-0372-8567-0e02b2c3d479;1234567890;1")),
		},
		{
			name: "success with nil uuid and invalid direction",
			fields: fields{
				Limit: 5,
			},
			args: args{
				id:        uuid.Nil(),
				serial:    42,
				direction: TokenDirectionInvalid,
			},
			want: base64.StdEncoding.EncodeToString([]byte("00000000-0000-0000-0000-000000000000;42;3")),
		},
		{
			name: "success with negative serial and next direction",
			fields: fields{
				Limit: 100,
			},
			args: args{
				id:        uuid.MustParse("a1a2a3a4-b1b2-c1c2-d1d2-d3d4d5d6d7d8"),
				serial:    -9876543210,
				direction: TokenDirectionNext,
			},
			want: base64.StdEncoding.EncodeToString([]byte("a1a2a3a4-b1b2-c1c2-d1d2-d3d4d5d6d7d8;-9876543210;2")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := &Paginator{
				NextToken: tt.fields.NextToken,
				NextPage:  tt.fields.NextPage,
				PrevToken: tt.fields.PrevToken,
				PrevPage:  tt.fields.PrevPage,
				Size:      tt.fields.Size,
				Limit:     tt.fields.Limit,
			}
			if got := ref.GenerateToken(tt.args.id, tt.args.serial, tt.args.direction); got != tt.want {
				t.Errorf("Paginator.GenerateToken() = %v, want %v", got, tt.want)
			}

			// Verify we can decode the token correctly
			id, serial, direction, err := DecodeToken(tt.want, tt.args.direction)

			if tt.args.direction == TokenDirectionInvalid {
				if err == nil {
					t.Errorf("DecodeToken() expected an error for TokenDirectionInvalid, but got nil")
				} else {
					// Optionally, check for a specific error message if desired, e.g.:
					expectedErrorMsg := "token contains an inherently invalid direction"
					if !strings.Contains(err.Error(), expectedErrorMsg) {
						t.Errorf("DecodeToken() error = %v, want error containing '%s'", err, expectedErrorMsg)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Failed to decode token for direction %s: %v", tt.args.direction, err)
				}
				if id != tt.args.id {
					t.Errorf("Decoded ID = %v, want %v for direction %s", id, tt.args.id, tt.args.direction.String())
				}
				if serial != tt.args.serial {
					t.Errorf("Decoded serial = %v, want %v for direction %s", serial, tt.args.serial, tt.args.direction.String())
				}
				if direction != tt.args.direction {
					t.Errorf("Decoded direction = %v, want %v for direction %s", direction, tt.args.direction, tt.args.direction.String())
				}
			}
		})
	}
}

func TestPaginator_Validate(t *testing.T) {
	validUUID := uuid.NewV7()
	validPrevToken := EncodeToken(validUUID, 123, TokenDirectionPrev)
	validNextToken := EncodeToken(validUUID, 123, TokenDirectionNext)
	invalidToken := "invalid-token-not-base64"
	invalidBase64Token := base64.StdEncoding.EncodeToString([]byte("invalid-format"))

	tests := []struct {
		name         string
		p            Paginator
		wantErr      bool
		wantErrField string
		wantErrCode  string
		wantErrMsg   string
	}{
		{
			name: "valid paginator with default values",
			p: Paginator{
				Limit: PaginatorDefaultLimit,
			},
			wantErr: false,
		},
		{
			name: "valid paginator with tokens",
			p: Paginator{
				NextToken: validNextToken,
				PrevToken: validPrevToken,
				Limit:     PaginatorDefaultLimit,
			},
			wantErr: false,
		},
		{
			name: "invalid limit - below minimum",
			p: Paginator{
				Limit: PaginatorMinLimit - 1,
			},
			wantErr:      true,
			wantErrField: "limit",
			wantErrCode:  "INVALID_RANGE",
			wantErrMsg:   "limit must be between",
		},
		{
			name: "invalid limit - above maximum",
			p: Paginator{
				Limit: PaginatorMaxLimit + 1,
			},
			wantErr:      true,
			wantErrField: "limit",
			wantErrCode:  "INVALID_RANGE",
			wantErrMsg:   "limit must be between",
		},
		{
			name: "invalid next token - not base64",
			p: Paginator{
				NextToken: invalidToken,
				Limit:     PaginatorDefaultLimit,
			},
			wantErr:      true,
			wantErrField: "next_token",
			wantErrCode:  "INVALID_TOKEN",
			wantErrMsg:   "next token cannot be decoded",
		},
		{
			name: "invalid prev token - not base64",
			p: Paginator{
				PrevToken: invalidToken,
				Limit:     PaginatorDefaultLimit,
			},
			wantErr:      true,
			wantErrField: "prev_token",
			wantErrCode:  "INVALID_TOKEN",
			wantErrMsg:   "previous token cannot be decoded",
		},
		{
			name: "invalid next token - invalid format",
			p: Paginator{
				NextToken: invalidBase64Token,
				Limit:     PaginatorDefaultLimit,
			},
			wantErr:      true,
			wantErrField: "next_token",
			wantErrCode:  "INVALID_TOKEN",
			wantErrMsg:   "next token cannot be decoded",
		},
		{
			name: "invalid prev token - invalid format",
			p: Paginator{
				PrevToken: invalidBase64Token,
				Limit:     PaginatorDefaultLimit,
			},
			wantErr:      true,
			wantErrField: "prev_token",
			wantErrCode:  "INVALID_TOKEN",
			wantErrMsg:   "previous token cannot be decoded",
		},
		{
			name: "valid limit at minimum",
			p: Paginator{
				Limit: PaginatorMinLimit,
			},
			wantErr: false,
		},
		{
			name: "valid limit at maximum",
			p: Paginator{
				Limit: PaginatorMaxLimit,
			},
			wantErr: false,
		},
		{
			name: "multiple validation errors - invalid limit and tokens",
			p: Paginator{
				NextToken: invalidToken,
				PrevToken: invalidToken,
				Limit:     PaginatorMinLimit - 1,
			},
			wantErr:      true,
			wantErrField: "limit", // We'll check for at least the limit error
			wantErrCode:  "INVALID_RANGE",
			wantErrMsg:   "limit must be between",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Paginator.Validate(): got = '%v', want = '%v'", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				validationErrors, ok := errors.AsType[*ValidationErrors](err)
				if !ok {
					t.Errorf("Expected error of type *ValidationErrors, got %T", err)
					return
				}

				// Check that we have at least one error
				if len(validationErrors.Errors) == 0 {
					t.Errorf("Expected validation errors but got none")
					return
				}

				// Check if we have the expected error field and message
				found := false
				for _, validationErr := range validationErrors.Errors {
					if validationErr.Field == tt.wantErrField &&
						validationErr.Code == tt.wantErrCode &&
						strings.Contains(validationErr.Message, tt.wantErrMsg) {
						found = true
						break
					}
				}

				if !found {
					t.Errorf("Expected validation error with field '%s', code '%s', and message containing '%s', but not found in errors: %v",
						tt.wantErrField, tt.wantErrCode, tt.wantErrMsg, validationErrors.Errors)
				}
			}
		})
	}
}

func TestPaginator_GeneratePages(t *testing.T) {
	validUUID := uuid.NewV7()

	tests := []struct {
		name         string
		paginator    Paginator
		url          string
		wantNextPage string
		wantPrevPage string
	}{
		{
			name: "both tokens present",
			paginator: Paginator{
				NextToken: EncodeToken(validUUID, 123, TokenDirectionNext),
				PrevToken: EncodeToken(validUUID, 122, TokenDirectionPrev),
				Limit:     20,
			},
			url:          "https://api.example.com/resources",
			wantNextPage: "https://api.example.com/resources?next_token=" + EncodeToken(validUUID, 123, TokenDirectionNext) + "&limit=20",
			wantPrevPage: "https://api.example.com/resources?prev_token=" + EncodeToken(validUUID, 122, TokenDirectionPrev) + "&limit=20",
		},
		{
			name: "only next token present",
			paginator: Paginator{
				NextToken: EncodeToken(validUUID, 123, TokenDirectionNext),
				PrevToken: "",
				Limit:     10,
			},
			url:          "https://api.example.com/resources",
			wantNextPage: "https://api.example.com/resources?next_token=" + EncodeToken(validUUID, 123, TokenDirectionNext) + "&limit=10",
			wantPrevPage: "",
		},
		{
			name: "only prev token present",
			paginator: Paginator{
				NextToken: "",
				PrevToken: EncodeToken(validUUID, 122, TokenDirectionPrev),
				Limit:     30,
			},
			url:          "https://api.example.com/resources",
			wantNextPage: "",
			wantPrevPage: "https://api.example.com/resources?prev_token=" + EncodeToken(validUUID, 122, TokenDirectionPrev) + "&limit=30",
		},
		{
			name: "no tokens present",
			paginator: Paginator{
				NextToken: "",
				PrevToken: "",
				Limit:     50,
			},
			url:          "https://api.example.com/resources",
			wantNextPage: "",
			wantPrevPage: "",
		},
		{
			name: "with path parameters",
			paginator: Paginator{
				NextToken: EncodeToken(validUUID, 123, TokenDirectionNext),
				PrevToken: EncodeToken(validUUID, 122, TokenDirectionPrev),
				Limit:     15,
			},
			url:          "https://api.example.com/users/42/resources",
			wantNextPage: "https://api.example.com/users/42/resources?next_token=" + EncodeToken(validUUID, 123, TokenDirectionNext) + "&limit=15",
			wantPrevPage: "https://api.example.com/users/42/resources?prev_token=" + EncodeToken(validUUID, 122, TokenDirectionPrev) + "&limit=15",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the paginator to avoid modifying the test case
			p := tt.paginator

			// Call the method under test
			p.GeneratePages(tt.url)

			// Check that the next page URL is correct
			if p.NextPage != tt.wantNextPage {
				t.Errorf("GeneratePages() NextPage = %v, want %v", p.NextPage, tt.wantNextPage)
			}

			// Check that the prev page URL is correct
			if p.PrevPage != tt.wantPrevPage {
				t.Errorf("GeneratePages() PrevPage = %v, want %v", p.PrevPage, tt.wantPrevPage)
			}
		})
	}
}

func TestGetTokens(t *testing.T) {
	// Setup test data
	firstID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	firstSerial := int64(1000)
	lastID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	lastSerial := int64(9000)

	tests := []struct {
		name                      string
		sizeOnDisplayPage         int
		firstIDOnDisplayPage      uuid.UUID
		firstSerialOnDisplayPage  int64
		lastIDOnDisplayPage       uuid.UUID
		lastSerialOnDisplayPage   int64
		dirUsedToFetchThisPage    TokenDirection
		repoFoundMoreForNextQuery bool
		repoFoundMoreForPrevQuery bool
		wantNext                  string
		wantPrev                  string
	}{
		{
			name:                      "empty result",
			sizeOnDisplayPage:         0,
			firstIDOnDisplayPage:      firstID, // Irrelevant for empty result, but provided for consistency
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionInvalid,
			repoFoundMoreForNextQuery: false,
			repoFoundMoreForPrevQuery: false,
			wantNext:                  "",
			wantPrev:                  "",
		},
		{
			name:                      "initial load, full page, more next, no more prev",
			sizeOnDisplayPage:         10,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionInvalid,
			repoFoundMoreForNextQuery: true,
			repoFoundMoreForPrevQuery: false,
			wantNext:                  EncodeToken(lastID, lastSerial, TokenDirectionNext),
			wantPrev:                  "",
		},
		{
			name:                      "initial load, partial page, no more next, no more prev",
			sizeOnDisplayPage:         5,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionInvalid,
			repoFoundMoreForNextQuery: false,
			repoFoundMoreForPrevQuery: false,
			wantNext:                  "",
			wantPrev:                  "",
		},
		{
			name:                      "fetched next, full page, more next, more prev",
			sizeOnDisplayPage:         10,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionNext,
			repoFoundMoreForNextQuery: true,
			repoFoundMoreForPrevQuery: true, // This specific prev flag doesn't alter outcome for prev when dir is Next
			wantNext:                  EncodeToken(lastID, lastSerial, TokenDirectionNext),
			wantPrev:                  EncodeToken(firstID, firstSerial, TokenDirectionPrev),
		},
		{
			name:                      "fetched next, partial page, no more next, more prev (last page going forward)",
			sizeOnDisplayPage:         5,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionNext,
			repoFoundMoreForNextQuery: false,
			repoFoundMoreForPrevQuery: true,
			wantNext:                  "",
			wantPrev:                  EncodeToken(firstID, firstSerial, TokenDirectionPrev),
		},
		{
			name:                      "fetched prev, full page, more next, more prev",
			sizeOnDisplayPage:         10,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionPrev,
			repoFoundMoreForNextQuery: true, // This specific next flag doesn't alter outcome for next when dir is Prev
			repoFoundMoreForPrevQuery: true,
			wantNext:                  EncodeToken(lastID, lastSerial, TokenDirectionNext),
			wantPrev:                  EncodeToken(firstID, firstSerial, TokenDirectionPrev),
		},
		{
			name:                      "fetched prev, partial page, more next, no more prev (first page going backward)",
			sizeOnDisplayPage:         5,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionPrev,
			repoFoundMoreForNextQuery: true,
			repoFoundMoreForPrevQuery: false,
			wantNext:                  EncodeToken(lastID, lastSerial, TokenDirectionNext),
			wantPrev:                  "",
		},
		{
			name:                      "initial load, full page, no more next, no more prev (single full page)",
			sizeOnDisplayPage:         10,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionInvalid,
			repoFoundMoreForNextQuery: false,
			repoFoundMoreForPrevQuery: false,
			wantNext:                  "",
			wantPrev:                  "",
		},
		{
			name:                      "fetched next, full page, no more next (at the end)",
			sizeOnDisplayPage:         10,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionNext,
			repoFoundMoreForNextQuery: false,
			repoFoundMoreForPrevQuery: true,
			wantNext:                  "",
			wantPrev:                  EncodeToken(firstID, firstSerial, TokenDirectionPrev),
		},
		{
			name:                      "fetched prev, full page, no more prev (at the beginning)",
			sizeOnDisplayPage:         10,
			firstIDOnDisplayPage:      firstID,
			firstSerialOnDisplayPage:  firstSerial,
			lastIDOnDisplayPage:       lastID,
			lastSerialOnDisplayPage:   lastSerial,
			dirUsedToFetchThisPage:    TokenDirectionPrev,
			repoFoundMoreForNextQuery: true,
			repoFoundMoreForPrevQuery: false,
			wantNext:                  EncodeToken(lastID, lastSerial, TokenDirectionNext),
			wantPrev:                  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, prev := GetTokens(
				tt.sizeOnDisplayPage,
				tt.firstIDOnDisplayPage,
				tt.firstSerialOnDisplayPage,
				tt.lastIDOnDisplayPage,
				tt.lastSerialOnDisplayPage,
				tt.dirUsedToFetchThisPage,
				tt.repoFoundMoreForNextQuery,
				tt.repoFoundMoreForPrevQuery,
			)

			if next != tt.wantNext {
				t.Errorf("GetTokens() next = %v, want %v", next, tt.wantNext)
			}
			if prev != tt.wantPrev {
				t.Errorf("GetTokens() prev = %v, want %v", prev, tt.wantPrev)
			}

			if tt.wantNext != "" {
				// Decode and verify next token contents
				decodedID, decodedSerial, decodedDir, err := DecodeToken(next, TokenDirectionNext)
				if err != nil {
					t.Errorf("Failed to decode next token: %v", err)
				}
				if decodedID != tt.lastIDOnDisplayPage {
					t.Errorf("Next token ID = %v, want %v", decodedID, tt.lastIDOnDisplayPage)
				}
				if decodedSerial != tt.lastSerialOnDisplayPage {
					t.Errorf("Next token serial = %v, want %v", decodedSerial, tt.lastSerialOnDisplayPage)
				}
				if decodedDir != TokenDirectionNext {
					t.Errorf("Next token direction = %v, want %v", decodedDir, TokenDirectionNext)
				}
			}

			if tt.wantPrev != "" {
				// Decode and verify prev token contents
				decodedID, decodedSerial, decodedDir, err := DecodeToken(prev, TokenDirectionPrev)
				if err != nil {
					t.Errorf("Failed to decode prev token: %v", err)
				}
				if decodedID != tt.firstIDOnDisplayPage {
					t.Errorf("Prev token ID = %v, want %v", decodedID, tt.firstIDOnDisplayPage)
				}
				if decodedSerial != tt.firstSerialOnDisplayPage {
					t.Errorf("Prev token serial = %v, want %v", decodedSerial, tt.firstSerialOnDisplayPage)
				}
				if decodedDir != TokenDirectionPrev {
					t.Errorf("Prev token direction = %v, want %v", decodedDir, TokenDirectionPrev)
				}
			}
		})
	}
}

func TestGetPaginatorDirection(t *testing.T) {
	validUUID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	validSerialNumber := int64(1234)
	validNextToken := EncodeToken(validUUID, validSerialNumber, TokenDirectionNext)
	validPrevToken := EncodeToken(validUUID, validSerialNumber, TokenDirectionPrev)
	invalidToken := "invalid-token"

	tests := []struct {
		name        string
		nextToken   string
		prevToken   string
		wantDir     TokenDirection
		wantID      uuid.UUID
		wantSerial  int64
		wantErr     bool
		errContains string
	}{
		{
			name:        "both tokens provided",
			nextToken:   validNextToken,
			prevToken:   validPrevToken,
			wantDir:     TokenDirectionInvalid,
			wantID:      uuid.Nil(),
			wantSerial:  0,
			wantErr:     true,
			errContains: "both next and prev tokens cannot be provided",
		},
		{
			name:       "only next token provided",
			nextToken:  validNextToken,
			prevToken:  "",
			wantDir:    TokenDirectionNext,
			wantID:     validUUID,
			wantSerial: validSerialNumber,
			wantErr:    false,
		},
		{
			name:       "only prev token provided",
			nextToken:  "",
			prevToken:  validPrevToken,
			wantDir:    TokenDirectionPrev,
			wantID:     validUUID,
			wantSerial: validSerialNumber,
			wantErr:    false,
		},
		{
			name:       "no tokens provided",
			nextToken:  "",
			prevToken:  "",
			wantDir:    TokenDirectionInvalid,
			wantID:     uuid.Nil(),
			wantSerial: 0,
			wantErr:    false,
		},
		{
			name:        "invalid next token",
			nextToken:   invalidToken,
			prevToken:   "",
			wantDir:     TokenDirectionInvalid,
			wantID:      uuid.Nil(),
			wantSerial:  0,
			wantErr:     true,
			errContains: "invalid token",
		},
		{
			name:        "invalid prev token",
			nextToken:   "",
			prevToken:   invalidToken,
			wantDir:     TokenDirectionInvalid,
			wantID:      uuid.Nil(),
			wantSerial:  0,
			wantErr:     true,
			errContains: "invalid token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, gotID, gotSerial, gotErr := GetPaginatorDirection(tt.nextToken, tt.prevToken)

			if (gotErr != nil) != tt.wantErr {
				t.Errorf("GetPaginatorDirection() error = %v, wantErr %v", gotErr, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errContains != "" && (gotErr == nil || !strings.Contains(gotErr.Error(), tt.errContains)) {
				t.Errorf("GetPaginatorDirection() error doesn't contain expected message, got = %v, want to contain = %v",
					gotErr, tt.errContains)
				return
			}

			if gotDir != tt.wantDir {
				t.Errorf("GetPaginatorDirection() direction = %v, want %v", gotDir, tt.wantDir)
			}

			if gotID != tt.wantID {
				t.Errorf("GetPaginatorDirection() id = %v, want %v", gotID, tt.wantID)
			}

			if gotSerial != tt.wantSerial {
				t.Errorf("GetPaginatorDirection() serial = %v, want %v", gotSerial, tt.wantSerial)
			}
		})
	}
}

func TestTokenDirection(t *testing.T) {
	tests := []struct {
		name      string
		direction TokenDirection
		wantStr   string
		wantValid bool
	}{
		{
			name:      "Prev direction",
			direction: TokenDirectionPrev,
			wantStr:   "prev",
			wantValid: true,
		},
		{
			name:      "Next direction",
			direction: TokenDirectionNext,
			wantStr:   "next",
			wantValid: true,
		},
		{
			name:      "Invalid direction",
			direction: TokenDirectionInvalid,
			wantStr:   "invalid",
			wantValid: false,
		},
		{
			name:      "Unknown direction",
			direction: TokenDirection(99),
			wantStr:   "unknown",
			wantValid: false,
		},
		{
			name:      "Zero direction",
			direction: TokenDirection(0),
			wantStr:   "unknown",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test String() method
			if got := tt.direction.String(); got != tt.wantStr {
				t.Errorf("TokenDirection.String() = %v, want %v", got, tt.wantStr)
			}

			// Test IsValid() method
			if got := tt.direction.IsValid(); got != tt.wantValid {
				t.Errorf("TokenDirection.IsValid() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}
