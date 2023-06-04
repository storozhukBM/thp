//go:build loong64
// +build loong64

#include "funcdata.h"
#include "go_asm.h"
#include "textflag.h"

TEXT ·getg(SB), NOSPLIT, $0-8
    MOVV    g, R8
    MOVV    R8, ret+0(FP)
    RET
