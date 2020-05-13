#include <u.h.>
#include <libc.h>

// while offset behavior for write is not specified in read(2) or read(5)
// the Plan 9 behaviour matches that of SUS seek/write
// contents: `0000 0000 7465 7374`
void main() {
	int fd = create("testfile", ORDWR, 0754);

	/*
	vlong cursor = seek(fd, 4, 1);
	print("cursor at: %d\n", cursor);
	long written = write(fd, "test", 4);
	*/
	// ↑ same as ↓ atomically
	long written = pwrite(fd,"test", 4, 4);
	print("wrote: %d\n", written);
}
