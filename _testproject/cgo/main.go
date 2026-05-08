package main

/*
#include <stdio.h>
#include <stdlib.h>
*/
import "C"
import "unsafe"

func main() {
	s := C.CString("hello from cgo")
	defer C.free(unsafe.Pointer(s))

	C.puts(s)
	C.fflush(C.stdout)
}
