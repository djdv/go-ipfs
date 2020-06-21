package filesystem

type (
	Kind uint8

	Error interface {
		error
		Kind() Kind
	}
)

// TODO: put a remark about this somewhere; probably in /transform/filesystems/???.go docs
// the intermediate operations that uses these errors aren't exactly the most truthful
// Kind biases towards POSIX errors in intermediate operations
// an example of this is ErrorPermission being returned from intermediate.Remove when called on the wrong type
// despite the fact this should really be ErrorInvalid
// another is ErrorOther being returned in Info for the same reason

//go:generate stringer -type=Kind -trimprefix=Error
const ( // kind subset kindly borrowed from rob
	ErrorOther            Kind = iota // Unclassified error.
	ErrorInvalidItem                  // Invalid operation for the item being operated on.
	ErrorInvalidOperation             // Operation itself is not valid within the system.
	ErrorPermission                   // Permission denied.
	ErrorIO                           // External I/O error such as network failure.
	ErrorExist                        // Item already exists.
	ErrorNotExist                     // Item does not exist.
	ErrorIsDir                        // Item is a directory.
	ErrorNotDir                       // Item is not a directory.
	ErrorNotEmpty                     // Directory not empty.
	ErrorReadOnly                     // File system has no modification capabilities.
)
