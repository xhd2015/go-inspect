# File copy
The filecopy pakcage does filewalk and copy files from one place to another.

It seems trivil, but the initial implementation got wrong.

Original implementation uses a concurrent file walk to copy files.
It blocks for large amount of files. The reason is that the consuming goroutine also writes directly to the channel, causing all goroutines to block when there is many more files to consume than it can consume. 

After done some benchmark on file io(see [https://github.com/xhd2015/bench-file-io](https://github.com/xhd2015/bench-file-io)), I decided to rewrite the implentation in a way that walk directory in one goroutine, and only send files for other goroutines to consume, that separates producing and consuming.