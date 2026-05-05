# Bilinovel Downloader

这是一个用于从 Bilinovel 下载和生成轻小说 EPUB 电子书的工具。
生成的 EPUB 文件完全符合 EPUB 标准，可以在 Calibre 检查中无错误通过。

## 使用示例

1. 下载整本 `https://www.bilinovel.com/novel/2388.html`

   ```bash
   bilinovel-downloader download -n 2388
   ```

   常用选项：
   - `--output-path`：最终 EPUB 输出目录，默认 `novels`
   - `--aux-path`：JSON 缓存、图片、EPUB 临时展开目录等辅助文件目录，默认与 `--output-path` 相同
   - `--clean-aux`：EPUB 生成成功后删除辅助文件，默认 `false`
   - `--output-type`：输出格式，默认 `epub`
   - `--concurrency`：整本下载时卷并发数，默认 `3`

2. 下载单卷 `https://www.bilinovel.com/novel/2388/vol_84522.html`

   ```bash
   bilinovel-downloader download -n 2388 -v 84522
   ```

3. 启动长期运行的下载命令服务

   ```bash
   DOWNLOAD_DIR=/downloads bilinovel-downloader server
   ```

   - `GET/POST /download/{novel_id}` 创建整本小说下载任务，返回 `202` 和 `job_id`
   - `GET/POST /download/{novel_id}/{vol_id}` 创建单卷下载任务，返回 `202` 和 `job_id`
   - 小说不存在时返回 `400 {"status":"error","message":"novel not found"}`
   - `GET /job/{job_id}` 查询任务状态
   - `DELETE /job/{job_id}` 取消排队中或运行中的任务

   服务配置：
   - `DOWNLOAD_DIR`：最终 EPUB 输出目录，默认 `./novels`（相对于服务进程当前工作目录）；Docker 镜像默认 `DOWNLOAD_DIR=/downloads`
   - `PLAYWRIGHT_MCP_EXECUTABLE_PATH`：Playwright 浏览器可执行文件路径，默认不设置，由 Playwright 自行解析
   - `AUX_DIR`：JSON 缓存、图片、EPUB 临时展开目录等辅助文件目录，默认与 `DOWNLOAD_DIR` 相同；Docker 镜像默认 `AUX_DIR=/aux`
   - `CLEAN_AUX_FILES`：EPUB 生成成功后删除辅助文件，默认 `false`
   - `SERVER_ADDR`：监听地址，默认 `:8080`
   - EPUB 输出到 `$DOWNLOAD_DIR/$NOVEL_TITLE/$VOLUME.epub`

4. 对自动生成的 epub 格式不满意可以自行修改后使用命令打包

   ```bash
   bilinovel-downloader pack -d <目录路径>
   ```

## 算法分析

目前程序使用 playwright 进行爬取来规避 bilinovel 的反爬（诱饵段落和段落重排）策略。  
但是依然对 bilinovel 的算法进行了简单的分析，具体可以参考[代码](./test/no_playwright_method_test.go)，这个代码目前是可行的，但如果 bilinovel 频繁更改初始化种子的计算方式或算法的实现，会让排序方法失效，这也是为什么目前程序使用 playwright。
