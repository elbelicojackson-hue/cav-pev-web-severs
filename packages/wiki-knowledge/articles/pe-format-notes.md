---
title: "PE文件格式笔记整理"
tags: ["reverse-engineering", "pe", "windows", "binary-analysis"]
source: "1nformation/Notes"
created: "2026-04-30"
updated: "2026-04-30"
---

# PE文件格式笔记整理

## PE整体结构

```
+----------------------+
|     DOS头 (MZ)       |  IMAGE_DOS_HEADER (0x40)
+----------------------+
|     DOS Stub         |  DOS存根程序
+----------------------+
|  NT头 (PE\0\0)       |  IMAGE_NT_HEADERS
|  +-- Signature       |  "PE\0\0" (0x00004550)
|  +-- FileHeader      |  IMAGE_FILE_HEADER (0x14)
|  +-- OptionalHeader  |  IMAGE_OPTIONAL_HEADER
+----------------------+
|     节表              |  IMAGE_SECTION_HEADER[]
+----------------------+
|     .text            |  代码节
+----------------------+
|     .rdata           |  只读数据（导入表等）
+----------------------+
|     .data            |  已初始化数据
+----------------------+
|     .bss             |  未初始化数据
+----------------------+
```

## DOS头

```c
typedef struct _IMAGE_DOS_HEADER {
    WORD e_magic;      // 0x00: "MZ" (0x5A4D) 标识
    // ... 中间成员兼容16位，可忽略 ...
    LONG e_lfanew;     // 0x3C: NT头偏移（默认0xB0）
} IMAGE_DOS_HEADER;
```

- e_magic: PE文件标识，必须是 "MZ" (0x4D5A)
- e_lfanew: 指向NT头的文件偏移

## 文件头 IMAGE_FILE_HEADER

```c
typedef struct _IMAGE_FILE_HEADER {
    WORD  Machine;              // CPU平台：x86=0x014C, x64=0x8664
    WORD  NumberOfSections;     // 节的数量
    DWORD TimeDateStamp;        // 时间戳
    DWORD PointerToSymbolTable; // 符号表位置
    DWORD NumberOfSymbols;      // 符号数量
    WORD  SizeOfOptionalHeader; // 选项头大小（用于定位节表）
    WORD  Characteristics;      // 文件属性
} IMAGE_FILE_HEADER;
```

Characteristics 标志:
- 0x0002: 可执行文件
- 0x0020: 支持大地址（>2GB）
- 0x0100: 32位程序
- 0x2000: DLL文件

## 选项头 IMAGE_OPTIONAL_HEADER

关键字段:
- Magic: 32位=0x10B, 64位=0x20B
- AddressOfEntryPoint (+0x10): 程序入口RVA（OEP）
- ImageBase (+0x1C): 建议加载基址（exe=0x400000, dll=0x10000000）
- SectionAlignment (+0x20): 内存对齐值（默认0x1000）
- FileAlignment (+0x24): 文件对齐值（默认0x200）
- SizeOfImage (+0x38): 内存中总大小
- SizeOfHeaders (+0x3C): 头部总大小
- DataDirectory (+0x60): 数据目录表（16项）

OEP/EP 概念:
- OEP (Original Entry Point): 原始程序入口点（RVA）
- EP (Entry Point): 被加工后的入口点（加壳后）
- 实际地址 = ImageBase + OEP

数据目录表索引:
- 0: 导出表
- 1: 导入表
- 2: 资源表
- 3: 异常表
- 5: 重定位表
- 9: TLS表
- 12: 导入地址表(IAT)

## 节表 IMAGE_SECTION_HEADER

```c
typedef struct _IMAGE_SECTION_HEADER {
    BYTE  Name[8];           // 节名称
    DWORD VirtualSize;       // 内存中有效数据大小
    DWORD VirtualAddress;    // 内存中RVA（与SectionAlignment对齐）
    DWORD SizeOfRawData;     // 文件中大小（与FileAlignment对齐）
    DWORD PointerToRawData;  // 文件中偏移
    DWORD Characteristics;   // 节属性
} IMAGE_SECTION_HEADER;
```

节属性:
- 0x00000020: 包含代码
- 0x00000040: 包含已初始化数据
- 0x20000000: 可执行
- 0x40000000: 可读
- 0x80000000: 可写

常见节:
- .text: 代码（可执行+可读）
- .rdata: 只读数据/导入表
- .data: 全局变量（可读写）
- .bss: 未初始化数据
- .rsrc: 资源
- .reloc: 重定位表

## 导入表

```c
typedef struct _IMAGE_IMPORT_DESCRIPTOR {
    DWORD OriginalFirstThunk; // INT RVA
    DWORD TimeDateStamp;
    DWORD ForwarderChain;
    DWORD Name;               // DLL名称 RVA
    DWORD FirstThunk;         // IAT RVA
} IMAGE_IMPORT_DESCRIPTOR;
```

INT 与 IAT:
- INT (Import Name Table): 存储函数名/序号
- IAT (Import Address Table): 加载后存储函数实际地址
- 加载前 INT 和 IAT 内容相同
- 加载器根据 INT 查找函数地址，填入 IAT

## 导出表

```c
typedef struct _IMAGE_EXPORT_DIRECTORY {
    DWORD Name;               // DLL名称 RVA
    DWORD Base;               // 序号基数
    DWORD NumberOfFunctions;  // 导出函数数量
    DWORD NumberOfNames;      // 有名称的函数数量
    DWORD AddressOfFunctions;    // 函数地址表 RVA
    DWORD AddressOfNames;        // 函数名称表 RVA
    DWORD AddressOfNameOrdinals; // 序号表 RVA
} IMAGE_EXPORT_DIRECTORY;
```

## 重定位表

- 当 DLL 无法加载到建议基址时需要重定位
- 存储需要修改的地址偏移

```c
typedef struct _IMAGE_BASE_RELOCATION {
    DWORD VirtualAddress; // 页 RVA
    DWORD SizeOfBlock;    // 块大小
    WORD  TypeOffset[1];  // 类型(4位) + 偏移(12位)
} IMAGE_BASE_RELOCATION;
```

重定位类型:
- 0 (ABSOLUTE): 填充对齐（忽略）
- 3 (HIGHLOW): 32位重定位

## TLS表

- 线程本地存储（Thread Local Storage）
- TLS回调函数在 OEP 之前执行（可用于反调试）

## 资源表

- 三级结构：类型 -> 名称 -> 语言
- 包含图标、对话框、字符串、版本信息等
