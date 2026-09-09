import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// 全局常量
class AppConfig {
  static const String baseUrl = 'http://127.0.0.1:8080/api';
  /// 大文件阈值（超过则分片），与后端契约一致：100MB
  static const int largeFileBytes = 100 * 1024 * 1024;
  /// 分片大小：10MB
  static const int chunkBytes = 10 * 1024 * 1024;
  /// 分片并发数
  static const int chunkConcurrency = 3;
  /// 主色
  static const int primary = 0xFF2563EB;
}

/// 业务异常：后端 code != 0
class ApiException implements Exception {
  final int code;
  final String message;
  const ApiException(this.code, this.message);

  @override
  String toString() => message;
}

/// 统一响应结构 {code, message, data}
class ApiResult<T> {
  final int code;
  final String message;
  final T data;
  const ApiResult(this.code, this.message, this.data);
}

/// 认证会话持久化
class Session {
  static const _kToken = 'auth_token';
  static const _kUser = 'auth_user';

  static Future<String?> loadToken() async {
    final sp = await SharedPreferences.getInstance();
    return sp.getString(_kToken);
  }

  static Future<Map<String, dynamic>?> loadUser() async {
    final sp = await SharedPreferences.getInstance();
    final raw = sp.getString(_kUser);
    if (raw == null) return null;
    try {
      return Map<String, dynamic>.from(jsonDecode(raw) as Map);
    } catch (_) {
      return null;
    }
  }

  static Future<void> save(String token, Map<String, dynamic> user) async {
    final sp = await SharedPreferences.getInstance();
    await sp.setString(_kToken, token);
    await sp.setString(_kUser, jsonEncode(user));
  }

  static Future<void> clear() async {
    final sp = await SharedPreferences.getInstance();
    await sp.remove(_kToken);
    await sp.remove(_kUser);
  }
}

/// HTTP 客户端封装
class ApiClient {
  ApiClient._();

  static final ApiClient instance = ApiClient._();

  late final Dio dio = _createDio();
  String? token;

  Dio _createDio() {
    final d = Dio(BaseOptions(
      baseUrl: AppConfig.baseUrl,
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 60),
      sendTimeout: const Duration(seconds: 120),
      headers: {'Accept': 'application/json'},
    ));
    d.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) {
        if (token != null && token!.isNotEmpty) {
          options.headers['Authorization'] = 'Bearer $token';
        }
        handler.next(options);
      },
    ));
    return d;
  }

  /// 解析统一响应，code != 0 抛 ApiException
  T _unwrap<T>(Response<dynamic> resp) {
    final body = resp.data;
    if (body is Map<String, dynamic>) {
      final code = (body['code'] as num?)?.toInt() ?? -1;
      final message = (body['message'] as String?) ?? '未知错误';
      if (code != 0) {
        throw ApiException(code, message);
      }
      return body['data'] as T;
    }
    throw const ApiException(-1, '响应格式错误');
  }

  Future<Map<String, dynamic>> post(String path, Map<String, dynamic> body) async {
    final resp = await dio.post<Map<String, dynamic>>(path, data: body);
    return _unwrap(resp);
  }

  Future<Map<String, dynamic>> get(String path, {Map<String, dynamic>? query}) async {
    final resp = await dio.get<Map<String, dynamic>>(path, queryParameters: query);
    return _unwrap(resp);
  }

  /// multipart 上传（返回 data 字段）
  Future<Map<String, dynamic>> upload(
    String path,
    Map<String, dynamic> fields,
    void Function(int sent, int total)? onProgress,
  ) async {
    final form = FormData();
    fields.forEach((k, v) {
      if (v is MultipartFile) {
        form.files.add(MapEntry(k, v));
      } else {
        form.fields.add(MapEntry(k, v.toString()));
      }
    });
    final resp = await dio.post<Map<String, dynamic>>(
      path,
      data: form,
      onSendProgress: onProgress,
    );
    return _unwrap(resp);
  }

  /// 下载文件到本地
  Future<void> download(String path, String savePath,
      {Map<String, dynamic>? query, void Function(int, int)? onProgress}) async {
    await dio.download(
      path,
      savePath,
      queryParameters: query,
      onReceiveProgress: onProgress,
    );
  }
}

/// 格式化文件大小
String formatSize(num bytes) {
  if (bytes < 1024) return '$bytes B';
  if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
  if (bytes < 1024 * 1024 * 1024) {
    return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
  }
  return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
}
