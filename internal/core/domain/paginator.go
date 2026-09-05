package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"uuid"
)

const (
	PaginatorDefaultLimit int = 10
	PaginatorMinLimit     int = 1
	PaginatorMaxLimit     int = 1000
)

// DataSeparator is the separator used to separate the data in the cursor token.
var DataSeparator string = ";"

// TokenDirection represents the direction of the token.
type TokenDirection uint8

const (
	// TokenDirectionPrev indicates the previous token direction.
	TokenDirectionPrev TokenDirection = 1

	// TokenDirectionNext indicates the next token direction.
	TokenDirectionNext TokenDirection = 2

	// TokenDirectionInvalid indicates an invalid token direction.
	TokenDirectionInvalid TokenDirection = 3
)

func (d TokenDirection) String() string {
	switch d {
	case TokenDirectionPrev:
		return "prev"
	case TokenDirectionNext:
		return "next"
	case TokenDirectionInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

func (d TokenDirection) IsValid() bool {
	switch d {
	case TokenDirectionPrev, TokenDirectionNext:
		return true
	default:
		return false
	}
}

// Paginator represents a model.
//
//	@Description	Paginator represents a paginator.
type Paginator struct {
	NextToken string `json:"next_token" example:"ZmZmZmZmZmYtZmZmZi0tZmZmZmZmZmY=" format:"string"`                                                // Base64 encoded next page token
	NextPage  string `json:"next_page" example:"http://localhost:8080/users?next_token=ZmZmZmZmZmYtZmZmZi0tZmZmZmZmZmY=&limit=10" format:"string"` // URL for the next page
	PrevToken string `json:"prev_token" example:"ZmZmZmZmZmYtZmZmZi0tZmZmZmZmZmY=" format:"string"`                                                // Base64 encoded previous page token
	PrevPage  string `json:"prev_page" example:"http://localhost:8080/users?prev_token=ZmZmZmZmZmYtZmZmZi0tZmZmZmZmZmY=&limit=10" format:"string"` // URL for the previous page
	Size      int    `json:"size" example:"10" format:"int"`                                                                                       // Number of items in the current page
	Limit     int    `json:"limit" example:"10" format:"int"`                                                                                      // Maximum number of items per page
}

// String returns the string representation of the model.
func (ref *Paginator) String() string {
	limit := fmt.Sprintf("%d", ref.Limit)

	return fmt.Sprintf("Paginator{next: %s, next_token: %s, prev: %s, prev_token: %s, size: %d, limit: %s}",
		ref.NextPage,
		ref.NextToken,
		ref.PrevPage,
		ref.PrevToken,
		ref.Size,
		limit,
	)
}

// GenerateToken generates a token for the given id and date.
func (ref *Paginator) GenerateToken(id uuid.UUID, serial int64, dir TokenDirection) string {
	return EncodeToken(id, serial, dir)
}

// Validate validates the model.
func (ref *Paginator) Validate() error {
	var errs ValidationErrors

	// Validate pagination limits using the centralized function
	if err := ValidatePagination(ref.Limit, 0, ""); err != nil {
		errs.Add(err)
	}

	// Additional business-specific validation for limit bounds
	if ref.Limit < PaginatorMinLimit || ref.Limit > PaginatorMaxLimit {
		errs.Add(&ValidationError{
			Field:   FieldLimit,
			Message: fmt.Sprintf("limit must be between %d and %d", PaginatorMinLimit, PaginatorMaxLimit),
			Code:    "INVALID_RANGE",
		})
	}

	// Validate next token if provided
	if ref.NextToken != "" {
		if _, err := ValidateString(ref.NextToken, StringValidationOptions{
			MinLength:      1,
			MaxLength:      1000, // reasonable base64 token length
			TrimWhitespace: true,
			AllowEmpty:     false,
			NoControlChars: true,
			NoNullBytes:    true,
			FieldName:      FieldNextToken,
		}); err != nil {
			errs.Add(err)
		} else {
			// Additional business logic validation for token decoding
			_, _, _, err := DecodeToken(ref.NextToken, TokenDirectionNext)
			if err != nil {
				errs.Add(&ValidationError{
					Field:   FieldNextToken,
					Message: "next token cannot be decoded",
					Code:    "INVALID_TOKEN",
				})
			}
		}
	}

	// Validate previous token if provided
	if ref.PrevToken != "" {
		if _, err := ValidateString(ref.PrevToken, StringValidationOptions{
			MinLength:      1,
			MaxLength:      1000, // reasonable base64 token length
			TrimWhitespace: true,
			AllowEmpty:     false,
			NoControlChars: true,
			NoNullBytes:    true,
			FieldName:      FieldPrevToken,
		}); err != nil {
			errs.Add(err)
		} else {
			// Additional business logic validation for token decoding
			_, _, _, err := DecodeToken(ref.PrevToken, TokenDirectionPrev)
			if err != nil {
				errs.Add(&ValidationError{
					Field:   FieldPrevToken,
					Message: "previous token cannot be decoded",
					Code:    "INVALID_TOKEN",
				})
			}
		}
	}

	if errs.HasErrors() {
		return &errs
	}

	return nil
}

// GeneratePages generates the next and previous pages.
func (ref *Paginator) GeneratePages(url string) {
	if ref.NextToken != "" {
		ref.NextPage = url + "?next_token=" + ref.NextToken + "&limit=" + strconv.Itoa(ref.Limit)
	} else {
		ref.NextPage = ""
	}

	if ref.PrevToken != "" {
		ref.PrevPage = url + "?prev_token=" + ref.PrevToken + "&limit=" + strconv.Itoa(ref.Limit)
	} else {
		ref.PrevPage = ""
	}
}

// UniqueID generates a unique ID based on the Paginator's field values.
// It uses SHA-256 hashing of a formatted string representation of the fields
// to ensure a consistent and collision-resistant ID.
func (ref *Paginator) UniqueID() string {
	// 1. Create a new SHA-256 hash instance.
	//    The hash.Hash interface implements io.Writer.
	h := sha256.New()

	// 2. Write the fields to the hash function in a deterministic order.
	//    Using a separator (like a null byte '\x00' or another unambiguous character)
	//    prevents collisions like ("ab", "c") vs ("a", "bc").
	//    fmt.Fprintf is convenient as it writes formatted data directly to the io.Writer (the hasher).
	//    It automatically handles the conversion of integers to their string representation.
	fmt.Fprintf(h, "%s\x00%s\x00%d\x00%d",
		ref.NextToken,
		ref.PrevToken,
		ref.Size,
		ref.Limit)

	// 3. Get the resulting hash sum as a byte slice.
	//    h.Sum(nil) appends the hash to a new nil slice and returns it.
	hashBytes := h.Sum(nil)

	// 4. Encode the byte slice into a hexadecimal string.
	//    This provides a standard, readable string representation of the hash.
	return hex.EncodeToString(hashBytes)
}

// EncodeToken encodes uuid and serial number into a base64 string.
// It returns a base64 encoded string of the uuid and serial number.
// It uses the package variables DataSeparator and DateFormat
// to set the separator and the date format.
func EncodeToken(id uuid.UUID, serial int64, dir TokenDirection) string {
	payload := fmt.Sprintf("%s%s%d%s%d", id.String(), DataSeparator, serial, DataSeparator, dir)

	return base64.StdEncoding.EncodeToString([]byte(payload))
}

// DecodeToken decodes base64 string into a uuid and serial number.
func DecodeToken(s string, expectedDir TokenDirection) (id uuid.UUID, serial int64, actualDir TokenDirection, err error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return uuid.Nil(), 0, TokenDirectionInvalid, &InvalidCursorError{Message: "invalid token: not base64"}
	}

	parts := strings.Split(string(data), DataSeparator)
	if len(parts) != 3 {
		return uuid.Nil(), 0, TokenDirectionInvalid, &InvalidCursorError{Message: "invalid token: incorrect format"}
	}

	id, err = uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil(), 0, TokenDirectionInvalid, &InvalidCursorError{Message: "invalid token: invalid uuid"}
	}

	serial, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return uuid.Nil(), 0, TokenDirectionInvalid, &InvalidCursorError{Message: "invalid token: invalid serial number"}
	}

	tmpDirVal, err := strconv.Atoi(parts[2])
	if err != nil {
		return uuid.Nil(), 0, TokenDirectionInvalid, &InvalidCursorError{Message: "invalid token: non-integer direction"}
	}

	// Check if the numeric value corresponds to any defined TokenDirection constant.
	if tmpDirVal < int(TokenDirectionPrev) || tmpDirVal > int(TokenDirectionInvalid) { // Assumes Prev=1, Next=2, Invalid=3
		return uuid.Nil(), 0, TokenDirectionInvalid, &InvalidCursorError{Message: "invalid token: unknown direction value"}
	}

	parsedDir := TokenDirection(tmpDirVal)

	// A token is inherently invalid if its embedded direction is not a usable one
	// according to IsValid() (e.g., TokenDirectionInvalid itself).
	if !parsedDir.IsValid() {
		return uuid.Nil(), 0, TokenDirectionInvalid, &InvalidCursorError{Message: "token contains an inherently invalid direction"}
	}

	// If the embedded direction is usable, it must also match the expected contextual direction.
	if parsedDir != expectedDir {
		return uuid.Nil(), 0, TokenDirectionInvalid, &InvalidCursorError{Message: fmt.Sprintf("token direction mismatch: expected %s, got %s", expectedDir, parsedDir)}
	}

	return id, serial, parsedDir, nil
}

// GetTokens returns the next and previous tokens based on the length and limit conditions.
// size is the number of elements in the current page.
// limit is the maximum number of elements to return.
// firstID is the ID of the first element in the current page.
// firstSerial is the serial number of the first element in the current page.
// lastID is the ID of the last element in the current page.
// lastSerial  is the serial number of the last element in the current page.
// dirToFetchPage indicates which token (if any) was used to fetch the current set of items.
// repoFoundMoreForNextQuery is true if the repository, when querying for what would constitute a "next" page, found more items than the display limit.
// repoFoundMoreForPrevQuery is true if the repository, when querying for what would constitute a "previous" page, found more items than the display limit.
func GetTokens(
	sizeOnDisplayPage int,
	firstIDOnDisplayPage uuid.UUID,
	firstSerialOnDisplayPage int64,
	lastIDOnDisplayPage uuid.UUID,
	lastSerialOnDisplayPage int64,
	dirUsedToFetchThisPage TokenDirection,
	repoFoundMoreForNextQuery bool,
	repoFoundMoreForPrevQuery bool,
) (next string, prev string) {
	if sizeOnDisplayPage == 0 {
		// No items on the current page to display, so no next or previous tokens.
		return "", ""
	}

	// Determine NEXT token
	if dirUsedToFetchThisPage == TokenDirectionPrev {
		// If we navigated BACK to this page using a "prev" token,
		// there's always a NEXT token to go FORWARD to the page we conceptually "came from",
		// provided this page has content.
		next = EncodeToken(lastIDOnDisplayPage, lastSerialOnDisplayPage, TokenDirectionNext)
	} else { // Initial load (TokenDirectionInvalid) or navigated FORWARD (TokenDirectionNext)
		if repoFoundMoreForNextQuery {
			// Repo explicitly found more items for a 'next' page.
			next = EncodeToken(lastIDOnDisplayPage, lastSerialOnDisplayPage, TokenDirectionNext)
		} else {
			// No more items for a 'next' page.
			next = ""
		}
	}

	// Determine PREVIOUS token
	if dirUsedToFetchThisPage == TokenDirectionNext {
		// If we navigated FORWARD to this page using a "next" token,
		// there's always a PREVIOUS token to go BACK to the page we conceptually "came from",
		// provided this page has content.
		prev = EncodeToken(firstIDOnDisplayPage, firstSerialOnDisplayPage, TokenDirectionPrev)
	} else { // Initial load (TokenDirectionInvalid) or navigated BACKWARD (TokenDirectionPrev)
		if repoFoundMoreForPrevQuery {
			// Repo explicitly found more items for a 'prev' page
			// (i.e., items before the current display set were confirmed to exist by the N+1 query).
			prev = EncodeToken(firstIDOnDisplayPage, firstSerialOnDisplayPage, TokenDirectionPrev)
		} else {
			// No more items for a 'prev' page (this is the true first page in this direction).
			prev = ""
		}
	}

	return next, prev
}

// GetPaginatorDirection returns the direction, id and serial number based on the next and previous tokens.
// It returns TokenDirectionNext if the next token is provided,
// TokenDirectionPrev if the previous token is provided.
// It returns an error if the token is invalid or if the uuid is invalid or if the serial number is invalid.
// The function uses the package variables DataSeparator and DateFormat
// to set the separator and the date format.
// The function also returns the id and serial number of the token.
func GetPaginatorDirection(nextToken string, prevToken string) (direction TokenDirection, id uuid.UUID, serial int64, err error) {
	// if both next and prev tokens are provided, use next token
	if nextToken != "" && prevToken != "" {
		return TokenDirectionInvalid, uuid.Nil(), 0, &InvalidCursorError{Message: "both next and prev tokens cannot be provided"}
	}

	if nextToken != "" {
		id, serial, direction, err := DecodeToken(nextToken, TokenDirectionNext)
		if err != nil {
			return TokenDirectionInvalid, uuid.Nil(), 0, err
		}

		return direction, id, serial, nil
	}

	if prevToken != "" {
		id, serial, direction, err := DecodeToken(prevToken, TokenDirectionPrev)
		if err != nil {
			return TokenDirectionInvalid, uuid.Nil(), 0, err
		}

		return direction, id, serial, nil
	}

	return TokenDirectionInvalid, uuid.Nil(), 0, nil
}
