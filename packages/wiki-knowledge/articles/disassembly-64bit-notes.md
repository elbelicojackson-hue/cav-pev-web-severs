---
title: "64位反汇编笔记整理"
tags: ["reverse-engineering", "disassembly", "x64", "c++"]
source: "1nformation/Notes"
created: "2026-04-30"
updated: "2026-04-30"
---

# 64位反汇编笔记整理

## 64位与32位的区别

### 程序层面
- 32位程序用32位编译器编译，64位用64位编译器
- 64位程序可访问 >4GB 地址空间
- 64位系统可运行32位程序（WoW64），反之不行
- 指针大小：32位=4字节，64位=8字节

### CPU层面
- 指令集不同：同一条指令可能有不同的机器码
- 寄存器数量和名称不同
- 64位没有指令周期，只有延时和吞吐量

### 手册
- Intel: 《Intel 64 and IA-32 Architectures Software Developer's Manual》
- AMD: 《AMD64 Architecture Programmer's Manual》

## x64 调用约定

### 唯一的调用约定：寄存器快速调用

- 前4个整数参数：RCX, RDX, R8, R9
- 前4个浮点参数：XMM0, XMM1, XMM2, XMM3
- 超过4个参数：从右到左压栈
- 调用者负责平衡栈空间
- 返回值：RAX（整数）/ XMM0（浮点）

### 栈帧结构

```
+------------------------+
| 第N个参数 (N>4)        |  调用者压栈
+------------------------+
| 第5个参数              |
+------------------------+
| 预留32字节 Shadow Space |  调用者分配
+------------------------+
| 返回地址               |  CALL 自动压入
+------------------------+
| 保存的 RBP             |  被调用者保存
+------------------------+
| 局部变量               |
+------------------------+
```

### 关键点

- 即使参数少于4个，也要预留32字节 Shadow Space
- Shadow Space 用于被调用者保存寄存器参数
- 所有参数如果要 push，必须在抬栈之前，且只有一次机会

## 64位寄存器

### 通用寄存器扩展

| 32位 | 64位 | 用途 |
|------|------|------|
| EAX | RAX | 返回值 |
| EBX | RBX | 被调用者保存 |
| ECX | RCX | 第1个参数 |
| EDX | RDX | 第2个参数 |
| ESI | RSI | 被调用者保存 |
| EDI | RDI | 被调用者保存 |
| ESP | RSP | 栈指针 |
| EBP | RBP | 帧指针 |
| - | R8 | 第3个参数 |
| - | R9 | 第4个参数 |
| - | R10-R11 | 调用者保存 |
| - | R12-R15 | 被调用者保存 |

### 寄存器保存约定

- 调用者保存（Caller-saved）: RAX, RCX, RDX, R8-R11
- 被调用者保存（Callee-saved）: RBX, RBP, RDI, RSI, R12-R15

## 64位指令特点

### 常见指令变化

- MOVSX 扩展到64位：`movsxd rax, ecx`
- 栈操作默认8字节：`push rbp` / `pop rbp`
- LEA 常用于地址计算：`lea rax, [rcx+rdx*4+8]`

### 浮点运算

- 使用 XMM/YMM 寄存器
- SSE 指令：movss, movsd, addss, addsd 等
- AVX 指令：vmovss, vaddss 等

## C++ 反汇编

### 构造函数与析构函数

- 构造函数：填充虚表指针，初始化成员
- 析构函数：清理资源，虚表指针可能被重置
- 编译器可能优化掉简单的构造/析构

### 虚函数（多态）

虚函数是识别面向对象的重要依据，不可被优化。

虚表结构:
- 对象首地址存储虚表指针（vptr）
- 虚表是一个函数指针数组
- 虚表中存储虚函数的地址

```cpp
class Base {
public:
    virtual void foo() { /* ... */ }
    virtual void bar() { /* ... */ }
};
// 对象布局: [vptr] [member1] [member2] ...
// 虚表: [&foo] [&bar]
```

虚函数调用:
```asm
; obj->foo()
mov rax, [rcx]        ; rax = vptr (对象首地址)
call [rax]            ; 调用虚表第一项 (foo)
; obj->bar()
mov rax, [rcx]
call [rax+8]          ; 调用虚表第二项 (bar)
```

### 继承

- 单继承：子类虚表在父类基础上追加
- 多重继承：每个基类有独立的虚表
- 虚继承：通过虚基类表访问共享基类

### 异常处理

- x64 使用基于表的异常处理（非 SEH 链）
- 异常处理信息存储在 .pdata 和 .xdata 节
- 使用 UNWIND_INFO 结构描述函数的栈帧

## 反汇编模式识别

### 条件分支

```asm
; if-else
cmp eax, 0
jne .else_branch
; then block
jmp .end_if
.else_branch:
; else block
.end_if:
```

### 循环

```asm
; for loop
xor ecx, ecx          ; i = 0
.loop_start:
cmp ecx, 10           ; i < 10
jge .loop_end
; loop body
inc ecx
jmp .loop_start
.loop_end:
```

### switch-case

- 少量 case: if-else 链
- 密集 case: 跳转表（jump table）
- 稀疏 case: 二分查找

### 数组访问

```asm
; arr[i] where arr is int[]
movsxd rax, ecx       ; sign-extend index
mov edx, [rsi+rax*4]  ; load arr[i]
```

### 结构体访问

```asm
; obj->field (offset 0x10)
mov rax, [rcx+10h]    ; access field at offset 0x10
```
