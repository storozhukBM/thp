//go:build mips64 || mips64le
// +build mips64 mips64le

#include "funcdata.h"
#include "go_asm.h"
#include "textflag.h"

TEXT ·getg(SB), NOSPLIT, $0-8
    MOVV    g, R8
    MOVV    R8, ret+0(FP)
    RET
