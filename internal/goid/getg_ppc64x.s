//go:build ppc64 || ppc64le
// +build ppc64 ppc64le

#include "funcdata.h"
#include "go_asm.h"
#include "textflag.h"

TEXT ·getg(SB), NOSPLIT, $0-8
    MOVD    g, R8
    MOVD    R8, ret+0(FP)
    RET
