---
title: "Win32汇编笔记整理"
tags: ["reverse-engineering", "assembly", "win32", "hook", "seh", "anti-debug"]
source: "1nformation/Notes"
created: "2026-04-30"
updated: "2026-04-30"
---

# Win32汇编笔记整理

## 编译与链接

### RadASM 开发环境

- RadASM 是 Windows 汇编 IDE
- 支持 MASM32 汇编器
- 项目文件结构：.rap（项目）、.asm（源码）、.inc（头文件）、.lib（库文件）

### 编译流程

```
.asm → masm → .obj → link → .exe
```

- 汇编器：ml.exe（MASM）
- 链接器：link.exe
- 资源编译器：rc.exe

## API Hook（内联钩子）

### 原理

修改 API 函数入口处的指令，使其跳转到我们的代码，执行完后跳回原流程。

### 实现步骤

1. 找到目标 API 入口地址
2. 保存入口处原始 5 字节（一条 JMP 指令的长度）
3. 修改入口处为 `JMP 我们的函数地址`
4. 在我们的函数中：
   - 执行自定义逻辑
   - 恢复原始 5 字节
   - 调用原 API
   - 再次修改入口为 JMP（可选）

### 关键点

- API 入口处通常有固定的 3 条指令（MOV EDI, EDI / PUSH EBP / MOV EBP, ESP），共 5 字节
- 这 5 字节恰好可以放一条 JMP 指令
- 很多 API 开头都有这个模式，是微软特意留的位置

### 汇编实现

```asm
; 1. 保存原始字节
invoke RtlMoveMemory, addr g_szOldBytes, lpApiAddr, 5

; 2. 写入 JMP
mov byte ptr [lpApiAddr], 0E9h        ; JMP opcode
mov eax, lpHookFunc
sub eax, lpApiAddr
sub eax, 5
mov dword ptr [lpApiAddr+1], eax      ; JMP offset

; 3. Hook函数中恢复并调用原API
invoke RtlMoveMemory, lpApiAddr, addr g_szOldBytes, 5
invoke MessageBox, ...  ; 调用原API
; 4. 重新Hook（可选）
```

## SEH（结构化异常处理）

### 概念

- SEH 是函数级别的异常处理（对比：筛选器是进程级别）
- 通过回调函数实现，注册给操作系统
- 类似 C++ 的 try/catch

### 注册方式

将异常回调函数地址存到 `FS:[0]`（异常链表头）

### 异常链表结构

```c
EXCEPTION_REGISTRATION_RECORD {
    Next    dd ?    ; 下一个异常处理结构体指针
    Handler dd ?    ; 当前异常回调函数地址
}
```

### 回调函数签名

```c
typedef EXCEPTION_DISPOSITION (*PEXCEPTION_ROUTINE)(
    PEXCEPTION_RECORD ExceptionRecord,  // 异常记录
    PVOID EstablisherFrame,             // 帧指针
    PCONTEXT Context,                   // 寄存器环境
    PVOID DispatcherContext             // 调度上下文
);
```

### 返回值

| 返回值 | 含义 |
|--------|------|
| ExceptionContinueExecution (0) | 继续执行异常指令 |
| ExceptionContinueSearch (1) | 不处理，交给下一个处理器 |

### 汇编实现

```asm
; 构造异常注册结构体
LOCAL @err:EXCEPTION_REGISTRATION_RECORD

; 保存原异常链
mov eax, fs:[0]
mov @err.Next, eax

; 注册新异常处理器
mov @err.Handler, offset MyExceptionHandler
lea eax, @err
mov fs:[0], eax

; ... 可能产生异常的代码 ...

; 恢复原异常链
mov eax, @err.Next
mov fs:[0], eax
```

### SEH 应用场景

1. 反调试：检测调试器的存在
2. 反跟踪：利用异常改变程序流程
3. 崩溃处理：优雅处理程序错误
4. 代码保护：隐藏真实逻辑

## 反调试技术

### 基于 SEH 的反调试

1. 故意触发异常
2. 正常运行时：SEH 处理异常，继续执行
3. 调试器存在时：调试器先捕获异常，SEH 不被调用

### 常见反调试手段

- `IsDebuggerPresent()` API 检测
- `CheckRemoteDebuggerPresent()` 检测
- PEB.BeingDebugged 标志检测
- 时间检测（执行时间异常）
- INT 2D 断点检测
- NtQueryInformationProcess 检测

## OD（OllyDbg）插件

### 常用插件功能

- **OllyDump**: 转储内存中的 PE 文件
- **StrongOD**: 反反调试插件
- **HideOD**: 隐藏调试器
- **Phantom**: 消除调试器痕迹
- **Script**: 自动化脚本执行

### 插件开发基础

- 基于 OD 的 Plugin API
- 可以在菜单中添加功能项
- 可以拦截 OD 的各种事件（断点、单步等）
