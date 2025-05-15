#include <stdio.h>
#include <unistd.h>

#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#include <Windows.h>
#else
#include <pthread.h>
#endif

#include "sapi/embed/php_embed.h"
#include "Zend/zend_exceptions.h"
#include "ext/phar/php_phar.h"

extern unsigned char payload_phar[];
extern unsigned int payload_phar_len;

static const size_t memory_limit = 1024 * 1024 * 1024; // 1 GB
static const size_t stack_limit  =   64 * 1024 * 1024; // 64 MB

static int eval(const char *str, size_t len, zval *retval) {
    zval script;
    ZVAL_STRINGL(&script, str, len);

    zend_op_array *op_array;
    op_array = zend_compile_string(&script, "script");
    zval_dtor(&script);
    if (!op_array) {
        return 1;
    }

    int failed = 0;
    zend_first_try {
        zend_execute(op_array, retval);
        if (EG(exception)) {
            failed = 1;
        }
    } zend_catch {
        failed = 1;
    } zend_end_try();

    destroy_op_array(op_array);
    efree(op_array);

    return failed;
}

static char *get_error_message(void) {
    if (EG(exception)) {
        zval exception_object, rv, *message;
        zend_class_entry *exception_ce = NULL;

        ZVAL_OBJ(&exception_object, EG(exception));
        if (instanceof_function(Z_OBJCE(exception_object), zend_ce_exception)) {
            exception_ce = zend_ce_exception;
        } else if (instanceof_function(Z_OBJCE(exception_object), zend_ce_error)) {
            exception_ce = zend_ce_error;
        }

        if (exception_ce) {
            message = zend_read_property_ex(exception_ce, &exception_object, ZSTR_KNOWN(ZEND_STR_MESSAGE), 1, &rv);
            if (Z_TYPE(*message) == IS_STRING) {
                return Z_STRVAL(*message);
            }
        }
    }
    return "unknown error";
}

static int init_php_stdio(int fd_in, int fd_out) {
    int fd_err = 2;

    php_stream *s_in, *s_out, *s_err;
    s_in  = php_stream_fopen_from_fd(fd_in,  "rb", NULL);
    s_out = php_stream_fopen_from_fd(fd_out, "wb", NULL);
    s_err = php_stream_fopen_from_fd(fd_err, "wb", NULL);

    if (!s_in || !s_out || !s_err) {
        if (s_in)  { php_stream_close(s_in);  }
        if (s_out) { php_stream_close(s_out); }
        if (s_err) { php_stream_close(s_err); }
        return 1;
    }

    s_in->flags  |= PHP_STREAM_FLAG_NO_BUFFER;
    s_out->flags |= PHP_STREAM_FLAG_NO_BUFFER;
    s_err->flags |= PHP_STREAM_FLAG_NO_BUFFER;

    // php only checks S_ISFIFO, which is not enough (e.g. tty)
    s_in->flags  |= PHP_STREAM_FLAG_NO_SEEK;
    s_out->flags |= PHP_STREAM_FLAG_NO_SEEK;
    s_err->flags |= PHP_STREAM_FLAG_NO_SEEK;

    zend_constant ic, oc, ec;
    php_stream_to_zval(s_in,  &ic.value);
    php_stream_to_zval(s_out, &oc.value);
    php_stream_to_zval(s_err, &ec.value);

    ic.flags = CONST_CS;
    ic.name = zend_string_init("STDIN", sizeof("STDIN")-1, 1);
    ic.module_number = 0;
    zend_register_constant(&ic);

    oc.flags = CONST_CS;
    oc.name = zend_string_init("STDOUT", sizeof("STDOUT")-1, 1);
    oc.module_number = 0;
    zend_register_constant(&oc);

    ec.flags = CONST_CS;
    ec.name = zend_string_init("STDERR", sizeof("STDERR")-1, 1);
    ec.module_number = 0;
    zend_register_constant(&ec);

    return 0;
}

static int load_phar(void) {
    php_stream *fp = php_stream_memory_open(TEMP_STREAM_READONLY, (char *)payload_phar, payload_phar_len);

    static const char fname[] = "payload.phar";
    static const char alias[] = "payload";
    phar_archive_data *pphar;
    if (phar_open_from_fp(fp, (char*)fname, sizeof fname - 1, (char*)alias, sizeof alias - 1, 0, &pphar, 0, NULL) != SUCCESS) {
        return 1;
    }
    phar_archive_addref(pphar);

    return 0;
}

int init(intptr_t fd_in, intptr_t fd_out) {
    php_embed_module.php_ini_ignore = 1;
    if (php_embed_init(0, NULL) != SUCCESS) { return 1; }

    EG(error_handling) = EH_THROW;

    PG(memory_limit) = memory_limit;
    if (zend_set_memory_limit(memory_limit) != SUCCESS) { return 1; }

#ifdef _WIN32
    // duplicate Windows file handle and convert to msvcrt file descriptor
    HANDLE hProcess = GetCurrentProcess();

    if (!DuplicateHandle(hProcess, fd_in, hProcess, &fd_in, 0, FALSE, DUPLICATE_SAME_ACCESS)) {
        return 1;
    }
    fd_in = _open_osfhandle(fd_in, _O_RDONLY | _O_NOINHERIT | _O_BINARY);
    if (fd_in == -1) {
        CloseHandle(fd_in);
        return 1;
    }

    if (!DuplicateHandle(hProcess, fd_out, hProcess, &fd_out, 0, FALSE, DUPLICATE_SAME_ACCESS)) {
        _close(fd_in);
        return 1;
    }
    fd_out = _open_osfhandle(fd_out, _O_WRONLY | _O_NOINHERIT | _O_BINARY);
    if (fd_out == -1) {
        CloseHandle(fd_out);
        _close(fd_in);
        return 1;
    }
#else
    fd_in = dup(fd_in);
    if (fd_in == -1) {
        return 1;
    }

    fd_out = dup(fd_out);
    if (fd_out == -1) {
        close(fd_in);
        return 1;
    }
#endif

    if (init_php_stdio(fd_in, fd_out)) {
        return 1;
    }

    if (load_phar()) {
        return 1;
    }

    return 0;
}

static void* _execute(void* arg) {
    // 这个函数的目标是运行 index.php 里的循环
    static const char entry_php[] = "require 'phar://payload/index.php';"; // 确保路径正确
    // fprintf(stderr, "PHP thread started, executing entry script...\n"); // 添加日志
    // eval 返回非0表示失败
    if (eval(entry_php, sizeof entry_php - 1, NULL)) {
        fprintf(stderr, "PHP entry script execution failed: %s\n", get_error_message());
        // 这里返回非 NULL 可能可以被 pthread_join 捕获，但简单起见返回 NULL
        
    }
    // fprintf(stderr, "PHP thread finishing execution.\n"); // 添加日志
    return NULL; // 线程结束
}

int execute(void) {
#ifdef WIN32
    // Windows 下的实现需要确认是否也是异步的，如果需要阻塞，
    // 可能需要不同的机制，或者直接在主线程调用 _execute
    fprintf(stderr, "Executing PHP directly on Windows thread.\n");
     _execute(NULL); // 假设 _execute 在 Windows 下是阻塞的或我们期望它阻塞
#else
    pthread_t thread;
    pthread_attr_t attr;
    int ret; // 用于接收 pthread 函数返回值

    // fprintf(stderr, "Initializing pthread attributes...\n");
    if ((ret = pthread_attr_init(&attr)) != 0) {
        fprintf(stderr, "pthread_attr_init failed: %d\n", ret);
        return 1; // 初始化属性失败
    }

    // fprintf(stderr, "Setting pthread stack size...\n");
    if ((ret = pthread_attr_setstacksize(&attr, stack_limit)) != 0) {
        fprintf(stderr, "pthread_attr_setstacksize failed: %d\n", ret);
        pthread_attr_destroy(&attr); // 清理属性对象
        return 1; // 设置栈大小失败
    }

    // 不要设置 PTHREAD_CREATE_DETACHED，使用默认的 joinable
    // if (pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED)) { ... }

    // fprintf(stderr, "Creating PHP execution thread...\n");
    if ((ret = pthread_create(&thread, &attr, _execute, NULL)) != 0) {
        fprintf(stderr, "pthread_create failed: %d\n", ret);
        pthread_attr_destroy(&attr); // 清理属性对象
        return 1; // 创建线程失败
    }

    // 销毁不再需要的属性对象
    pthread_attr_destroy(&attr);

    // fprintf(stderr, "Waiting for PHP execution thread to join...\n");
    // 等待 _execute 线程结束
    if ((ret = pthread_join(thread, NULL)) != 0) {
        // 等待线程出错（理论上不常见，除非线程 ID 无效）
        fprintf(stderr, "pthread_join failed: %d\n", ret);
        return 1; // 等待失败
    }

    // fprintf(stderr, "PHP execution thread joined. execute() function returning.\n");
#endif
    // 只有当 PHP 线程 (_execute) 真正结束后，execute() 才会返回
    return 0; // 可以根据 _execute 的结果返回不同的值，但这里简单返回 0
}
