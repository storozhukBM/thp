#include "funcdata.h"
#include "go_asm.h"
#include "textflag.h"

TEXT ·getg(SB), NOSPLIT, $0-8
    MOV    g, X10
    MOV    X10, ret+0(FP)
    RET
