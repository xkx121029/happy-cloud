import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';

import '../core/api.dart';
import 'app_state.dart';

/// 上传服务：计算 SHA256 → 秒传校验 → 小文件直传 / 大文件分片并发上传
class UploadService {
  UploadService._();

  static final UploadService instance = UploadService._();

  /// 计算文件 SHA256（流式计算，适合大文件）
  static Future<String> sha256Of(String path) async {
    final file = File(path);
    final digest = await sha256.bind(file.openRead()).reduce((a, b) => b);
    return digest.toString();
  }

  /// 上传单个文件，返回是否已秒传
  Future<bool> uploadFile(AppState state, UploadTask task) async {
    final file = File(task.path);
    final parentId = state.currentParentId;
    final name = task.name;
    final size = file.lengthSync();

    task.status = UploadStatus.hashing;
    task.progress = 0.02;
    state.refresh();

    final hash = await sha256Of(task.path);
    task.status = UploadStatus.checking;
    state.refresh();

    // 秒传校验
    Map<String, dynamic> check;
    try {
      check = await ApiClient.instance.post('/files/upload/hash', {
        'name': name,
        'size': size,
        'hash': hash,
        'parent_id': parentId,
      });
    } on ApiException catch (e) {
      // 秒传接口失败（例如未实现），回退为普通上传
      if (e.code != 0) {
        check = {'exists': false};
      } else {
        rethrow;
      }
    }
    final exists = check['exists'] == true;
    if (exists) {
      task.status = UploadStatus.done;
      task.progress = 1;
      state.refresh();
      return true;
    }

    if (size > AppConfig.largeFileBytes) {
      await _uploadChunked(state, task, file, name, size, hash, parentId);
    } else {
      await _uploadDirect(state, task, file, name, parentId);
    }
    return false;
  }

  /// 小文件直传
  Future<void> _uploadDirect(
    AppState state,
    UploadTask task,
    File file,
    String name,
    int parentId,
  ) async {
    task.status = UploadStatus.uploading;
    state.refresh();
    final bytes = await file.readAsBytes();
    final multipart = MultipartFile.fromBytes(bytes, filename: name);
    await ApiClient.instance.upload('/files/upload', {
      'file': multipart,
      'parent_id': parentId,
    }, (sent, total) {
      if (total > 0) {
        task.progress = 0.05 + 0.95 * (sent / total);
        state.refresh();
      }
    });
    task.status = UploadStatus.done;
    task.progress = 1;
    state.refresh();
  }

  /// 大文件分片并发上传
  Future<void> _uploadChunked(
    AppState state,
    UploadTask task,
    File file,
    String name,
    int size,
    String hash,
    int parentId,
  ) async {
    final totalChunks = (size + AppConfig.chunkBytes - 1) ~/ AppConfig.chunkBytes;
    var completed = 0;

    Future<void> uploadOne(int index) async {
      final start = index * AppConfig.chunkBytes;
      final len = (start + AppConfig.chunkBytes > size)
          ? size - start
          : AppConfig.chunkBytes;
      // 每个分片独立打开句柄读取，避免并发 seek 竞争
      final chunk = await _readRange(file, start, len);
      final multipart = MultipartFile.fromBytes(chunk, filename: 'chunk_$index');
      await ApiClient.instance.upload('/files/upload/chunk', {
        'file': multipart,
        'hash': hash,
        'chunk_index': index,
        'chunk_total': totalChunks,
      }, (sent, total) {
        // 每片进度按片数加权，粗略展示
        task.progress =
            0.05 + 0.90 * ((completed + (total > 0 ? sent / total : 1)) / totalChunks);
        state.refresh();
      });
      completed++;
      task.progress = 0.05 + 0.90 * (completed / totalChunks);
      state.refresh();
    }

    // 并发上传全部分片，任一失败则异常传播，不执行合并
    try {
      task.status = UploadStatus.uploading;
      state.refresh();
      await _runWithConcurrency(totalChunks, AppConfig.chunkConcurrency, uploadOne);
    } catch (_) {
      task.status = UploadStatus.failed;
      state.refresh();
      rethrow;
    }

    task.status = UploadStatus.merging;
    task.progress = 0.97;
    state.refresh();

    await ApiClient.instance.post('/files/upload/merge', {
      'hash': hash,
      'name': name,
      'size': size,
      'parent_id': parentId,
      'chunk_total': totalChunks,
    });

    task.status = UploadStatus.done;
    task.progress = 1;
    state.refresh();
  }

  /// 读取文件指定区间（独立句柄，可并发）
  static Future<List<int>> _readRange(File file, int start, int len) async {
    final raf = await file.open();
    try {
      await raf.setPosition(start);
      return await raf.read(len);
    } finally {
      await raf.close();
    }
  }

  /// 简单并发调度器
  Future<void> _runWithConcurrency(
    int count,
    int concurrency,
    Future<void> Function(int index) fn,
  ) async {
    var next = 0;
    Future<void> worker() async {
      while (true) {
        final index = next++;
        if (index >= count) break;
        await fn(index);
      }
    }

    await Future.wait(List.generate(concurrency, (_) => worker()));
  }
}
