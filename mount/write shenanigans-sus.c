#include <stdio.h>
#include <stdlib.h>
#include <fcntl.h>

#define pathBufferSize 1024

// A SUS compliant system should return 4 and result in a file of 8 bytes
// resizing during the call to write, not during lseek
// contents: `0000 0000 7465 7374`

void main(void) {
	char pathTarg[pathBufferSize] = "";
	{
		char *pPtr;
		if (pPtr = getenv("TEST_PATH_TARGET")) {
			strncpy(pathTarg, pPtr, pathBufferSize - 1);
		} else {
			strcpy(pathTarg, "testfile");
		}
	}

	printf("Create, Truncate, and Open %s...\n", pathTarg);
	int fd = open(pathTarg, O_RDWR|O_CREAT|O_TRUNC);
	lseek(fd, 4, SEEK_CUR);
	int written = write(fd, "test", 4);
	printf("Wrote %d\n", written);
	close(fd);
}
